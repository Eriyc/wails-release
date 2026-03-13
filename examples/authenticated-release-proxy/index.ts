import { fromBinary, fromJsonString, toBinary, toJson } from "@bufbuild/protobuf";

import type { PublicDeltaManifest } from "../../gen/ts/wailsrel/v1/delta_pb.ts";
import { PublicDeltaManifestSchema } from "../../gen/ts/wailsrel/v1/delta_pb.ts";
import type { PublicReleaseManifest } from "../../gen/ts/wailsrel/v1/release_pb.ts";
import { PublicReleaseManifestSchema } from "../../gen/ts/wailsrel/v1/release_pb.ts";

type GitHubAsset = {
  id: number;
  name: string;
  url: string;
  content_type?: string;
};

type GitHubRelease = {
  tag_name: string;
  assets: GitHubAsset[];
};

type CachedValue<T> = {
  expiresAt: number;
  value: T;
};

const repository = requiredEnv("GITHUB_REPOSITORY");
const githubToken = optionalEnv("GITHUB_TOKEN");
const authToken = (Bun.env.PROXY_AUTH_TOKEN || "").trim();
const apiBaseUrl = (Bun.env.GITHUB_API_BASE_URL || "https://api.github.com").trim().replace(/\/$/, "");
const host = (Bun.env.HOST || "127.0.0.1").trim();
const port = Number((Bun.env.PORT || "8787").trim());
const cacheTtlMs = Number((Bun.env.CACHE_TTL_MS || "60000").trim());

const cache: {
  latest?: CachedValue<GitHubRelease>;
  byTag: Map<string, CachedValue<GitHubRelease>>;
} = {
  byTag: new Map(),
};

function requiredEnv(name: string): string {
  const value = Bun.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function optionalEnv(name: string): string {
  return Bun.env[name]?.trim() || "";
}

function responseJSON(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body, null, 2), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function unauthorized(message = "unauthorized"): Response {
  return responseJSON({ error: message }, 401);
}

function notFound(message = "not found"): Response {
  return responseJSON({ error: message }, 404);
}

function badGateway(message = "bad gateway"): Response {
  return responseJSON({ error: message }, 502);
}

function githubHeaders(accept: string, request?: Request): HeadersInit {
  const headers: Record<string, string> = {
    accept,
    "user-agent": "wailsrel-contract-http-example",
  };
  if (githubToken) {
    headers.authorization = `Bearer ${githubToken}`;
  }
  const range = request?.headers.get("range");
  if (range) {
    headers.range = range;
  }
  return headers;
}

async function fetchGitHubJSON<T>(url: string): Promise<T> {
  const response = await fetch(url, {
    headers: githubHeaders("application/vnd.github+json"),
  });
  if (!response.ok) {
    throw new Error(`github request failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function fetchGitHubBytes(asset: GitHubAsset): Promise<Uint8Array> {
  const response = await fetch(asset.url, {
    headers: githubHeaders("application/octet-stream"),
    redirect: "follow",
  });
  if (!response.ok) {
    throw new Error(`asset fetch failed: ${response.status}`);
  }
  return new Uint8Array(await response.arrayBuffer());
}

async function latestRelease(): Promise<GitHubRelease> {
  const now = Date.now();
  if (cache.latest && cache.latest.expiresAt > now) {
    return cache.latest.value;
  }
  const release = await fetchGitHubJSON<GitHubRelease>(`${apiBaseUrl}/repos/${repository}/releases/latest`);
  cache.latest = { value: release, expiresAt: now + cacheTtlMs };
  cache.byTag.set(release.tag_name, { value: release, expiresAt: now + cacheTtlMs });
  return release;
}

async function releaseByTag(tag: string): Promise<GitHubRelease> {
  const now = Date.now();
  const cached = cache.byTag.get(tag);
  if (cached && cached.expiresAt > now) {
    return cached.value;
  }
  const release = await fetchGitHubJSON<GitHubRelease>(
    `${apiBaseUrl}/repos/${repository}/releases/tags/${encodeURIComponent(tag)}`,
  );
  cache.byTag.set(tag, { value: release, expiresAt: now + cacheTtlMs });
  return release;
}

function findAsset(release: GitHubRelease, name: string): GitHubAsset | undefined {
  return release.assets.find((asset) => asset.name === name);
}

function prefersProtobuf(request: Request): boolean {
  return (request.headers.get("accept") || "").includes("application/x-protobuf");
}

function proxyURL(origin: string, tag: string, assetName: string): string {
  return `${origin}/download/${encodeURIComponent(tag)}/${encodeURIComponent(assetName)}`;
}

function assetNameFromValue(value: string | undefined): string | undefined {
  if (!value) {
    return undefined;
  }
  try {
    const url = new URL(value);
    const parts = url.pathname.split("/");
    return parts[parts.length - 1] || undefined;
  } catch {
    const parts = value.split("/");
    return parts[parts.length - 1] || undefined;
  }
}

async function loadReleaseManifest(release: GitHubRelease): Promise<PublicReleaseManifest> {
  const asset = findAsset(release, "manifest.pb") || findAsset(release, "manifest.json");
  if (!asset) {
    throw new Error("manifest asset not found on the latest release");
  }
  const bytes = await fetchGitHubBytes(asset);
  if (asset.name.endsWith(".pb")) {
    return fromBinary(PublicReleaseManifestSchema, bytes);
  }
  return fromJsonString(PublicReleaseManifestSchema, new TextDecoder().decode(bytes));
}

async function loadDeltaManifest(release: GitHubRelease): Promise<PublicDeltaManifest> {
  const asset = findAsset(release, "delta-manifest.pb") || findAsset(release, "delta-manifest.json");
  if (!asset) {
    throw new Error("delta-manifest asset not found on the latest release");
  }
  const bytes = await fetchGitHubBytes(asset);
  if (asset.name.endsWith(".pb")) {
    return fromBinary(PublicDeltaManifestSchema, bytes);
  }
  return fromJsonString(PublicDeltaManifestSchema, new TextDecoder().decode(bytes));
}

function rewriteReleaseManifest(manifest: PublicReleaseManifest, tag: string, origin: string): PublicReleaseManifest {
  const copy = structuredClone(manifest);
  if (copy.release) {
    copy.release.tag = tag;
    copy.release.provider = "http";
  }
  for (const artifact of copy.artifacts) {
    if (artifact.assetName) {
      artifact.url = proxyURL(origin, tag, artifact.assetName);
    }
  }
  if (copy.delta) {
    copy.delta.manifestUrl = `${origin}/delta/manifest`;
  }
  for (const bundle of copy.frontendBundles) {
    const assetName = assetNameFromValue(bundle.url || bundle.assetKey);
    if (assetName) {
      bundle.url = proxyURL(origin, tag, assetName);
    }
  }
  return copy;
}

function rewriteDeltaManifest(manifest: PublicDeltaManifest, tag: string, origin: string): PublicDeltaManifest {
  const copy = structuredClone(manifest);
  for (const patch of copy.patches) {
    const assetName = assetNameFromValue(patch.patch || patch.assetKey);
    if (assetName) {
      patch.patch = proxyURL(origin, tag, assetName);
    }
  }
  return copy;
}

function manifestResponse(request: Request, manifest: PublicReleaseManifest): Response {
  if (prefersProtobuf(request)) {
    return new Response(toBinary(PublicReleaseManifestSchema, manifest), {
      headers: { "content-type": "application/x-protobuf" },
    });
  }
  return new Response(`${JSON.stringify(toJson(PublicReleaseManifestSchema, manifest), null, 2)}\n`, {
    headers: { "content-type": "application/json" },
  });
}

function deltaResponse(request: Request, manifest: PublicDeltaManifest): Response {
  if (prefersProtobuf(request)) {
    return new Response(toBinary(PublicDeltaManifestSchema, manifest), {
      headers: { "content-type": "application/x-protobuf" },
    });
  }
  return new Response(`${JSON.stringify(toJson(PublicDeltaManifestSchema, manifest), null, 2)}\n`, {
    headers: { "content-type": "application/json" },
  });
}

function isAuthorized(request: Request): boolean {
  if (!authToken) {
    return true;
  }
  return request.headers.get("authorization") === `Bearer ${authToken}`;
}

async function streamAsset(request: Request, asset: GitHubAsset): Promise<Response> {
  const response = await fetch(asset.url, {
    headers: githubHeaders("application/octet-stream", request),
    redirect: "follow",
  });
  if (!response.ok) {
    return badGateway(`github asset fetch failed: ${response.status}`);
  }
  const headers = new Headers();
  for (const name of [
    "content-type",
    "content-length",
    "content-disposition",
    "etag",
    "last-modified",
    "cache-control",
    "accept-ranges",
    "content-range",
  ]) {
    const value = response.headers.get(name);
    if (value) {
      headers.set(name, value);
    }
  }
  return new Response(response.body, {
    status: response.status,
    headers,
  });
}

function infoPage(): Response {
  return new Response(
    [
      "wailsrel http example server",
      "",
      "Routes:",
      "  GET /manifest",
      "  GET /delta/manifest",
      "  GET /frontend/catalog (501 in this example)",
      "  GET /download/:tag/:asset_name",
      "  GET /healthz",
      "",
      "Negotiation:",
      "  Accept: application/json",
      "  Accept: application/x-protobuf",
      "",
      "GitHub auth:",
      "  GITHUB_TOKEN is optional for public repositories",
    ].join("\n"),
    { headers: { "content-type": "text/plain; charset=utf-8" } },
  );
}

const server = Bun.serve({
  hostname: host,
  port,
  async fetch(request) {
    try {
      const url = new URL(request.url);

      if (url.pathname === "/") {
        return infoPage();
      }
      if (url.pathname === "/healthz") {
        return responseJSON({ ok: true, repository, cacheTtlMs, authRequired: authToken !== "" });
      }
      if (url.pathname === "/manifest") {
        const release = await latestRelease();
        const manifest = rewriteReleaseManifest(await loadReleaseManifest(release), release.tag_name, url.origin);
        return manifestResponse(request, manifest);
      }
      if (url.pathname === "/delta/manifest") {
        const release = await latestRelease();
        const manifest = rewriteDeltaManifest(await loadDeltaManifest(release), release.tag_name, url.origin);
        return deltaResponse(request, manifest);
      }
      if (url.pathname === "/frontend/catalog") {
        return responseJSON({ error: "frontend catalog signing is external and not implemented in this example" }, 501);
      }

      const downloadMatch = url.pathname.match(/^\/download\/([^/]+)\/([^/]+)$/);
      if (downloadMatch) {
        if (!isAuthorized(request)) {
          return unauthorized();
        }
        const tag = decodeURIComponent(downloadMatch[1] || "");
        const assetName = decodeURIComponent(downloadMatch[2] || "");
        const release = await releaseByTag(tag);
        const asset = findAsset(release, assetName);
        if (!asset) {
          return notFound(`asset ${assetName} not found on release ${tag}`);
        }
        return streamAsset(request, asset);
      }

      return notFound();
    } catch (error) {
      return badGateway(error instanceof Error ? error.message : String(error));
    }
  },
});

console.log(`wailsrel http example server listening on http://${server.hostname}:${server.port}`);
