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

type ReleaseManifest = {
  release?: {
    tag?: string;
    provider?: string;
  };
  artifacts?: Array<{
    asset_name?: string;
    url?: string;
  }>;
  delta?: {
    manifest_url?: string;
  };
  patches?: Array<{
    url?: string;
  }>;
  frontend_bundles?: Array<{
    url?: string;
  }>;
};

type DeltaManifest = {
  patches?: Array<{
    patch?: string;
  }>;
};

type CachedValue<T> = {
  expiresAt: number;
  value: T;
};

const repository = requiredEnv("GITHUB_REPOSITORY");
const githubToken = requiredEnv("GITHUB_TOKEN");
const authToken = requiredEnv("PROXY_AUTH_TOKEN");
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

function unauthorized(message = "unauthorized"): Response {
  return json({ error: message }, 401);
}

function notFound(message = "not found"): Response {
  return json({ error: message }, 404);
}

function badGateway(message = "bad gateway"): Response {
  return json({ error: message }, 502);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body, null, 2), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function githubHeaders(accept: string, request?: Request): HeadersInit {
  const headers: Record<string, string> = {
    accept,
    authorization: `Bearer ${githubToken}`,
    "user-agent": "wailsrel-authenticated-release-proxy",
  };
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

async function latestRelease(): Promise<GitHubRelease> {
  const now = Date.now();
  if (cache.latest && cache.latest.expiresAt > now) {
    return cache.latest.value;
  }
  const release = await fetchGitHubJSON<GitHubRelease>(
    `${apiBaseUrl}/repos/${repository}/releases/latest`,
  );
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

async function fetchAssetJSON<T>(asset: GitHubAsset): Promise<T> {
  const response = await fetch(asset.url, {
    headers: githubHeaders("application/octet-stream"),
    redirect: "follow",
  });
  if (!response.ok) {
    throw new Error(`asset fetch failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

function proxyURL(origin: string, tag: string, assetName: string): string {
  return `${origin}/download/${encodeURIComponent(tag)}/${encodeURIComponent(assetName)}`;
}

function filenameFromURL(value: string | undefined): string | undefined {
  if (!value) {
    return undefined;
  }
  try {
    const url = new URL(value);
    return url.pathname.split("/").pop() || undefined;
  } catch {
    return value.split("/").pop();
  }
}

function rewriteManifest(manifest: ReleaseManifest, tag: string, origin: string): ReleaseManifest {
  const copy = structuredClone(manifest);
  if (copy.release) {
    copy.release.tag = tag;
    copy.release.provider = "http";
  }

  for (const artifact of copy.artifacts || []) {
    if (artifact.asset_name) {
      artifact.url = proxyURL(origin, tag, artifact.asset_name);
    }
  }

  if (copy.delta) {
    copy.delta.manifest_url = `${origin}/delta/manifest.json`;
  }

  for (const patch of copy.patches || []) {
    const assetName = filenameFromURL(patch.url);
    if (assetName) {
      patch.url = proxyURL(origin, tag, assetName);
    }
  }

  for (const bundle of copy.frontend_bundles || []) {
    const assetName = filenameFromURL(bundle.url);
    if (assetName) {
      bundle.url = proxyURL(origin, tag, assetName);
    }
  }

  return copy;
}

function rewriteDeltaManifest(manifest: DeltaManifest, tag: string, origin: string): DeltaManifest {
  const copy = structuredClone(manifest);
  for (const patch of copy.patches || []) {
    const assetName = filenameFromURL(patch.patch);
    if (assetName) {
      patch.patch = proxyURL(origin, tag, assetName);
    }
  }
  return copy;
}

function isAuthorized(request: Request): boolean {
  const expected = `Bearer ${authToken}`;
  return request.headers.get("authorization") === expected;
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
      "wailsrel authenticated release proxy",
      "",
      "Routes:",
      "  GET /manifest.json",
      "  GET /delta/manifest.json",
      "  GET /download/:tag/:asset_name  (requires Authorization: Bearer <token>)",
      "  GET /healthz",
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
        return json({ ok: true, repository, cacheTtlMs });
      }

      if (url.pathname === "/manifest.json") {
        const release = await latestRelease();
        const asset = findAsset(release, "manifest.json");
        if (!asset) {
          return notFound("manifest.json not found on the latest release");
        }
        const manifest = await fetchAssetJSON<ReleaseManifest>(asset);
        return json(rewriteManifest(manifest, release.tag_name, url.origin));
      }

      if (url.pathname === "/delta/manifest.json") {
        const release = await latestRelease();
        const asset = findAsset(release, "delta-manifest.json");
        if (!asset) {
          return notFound("delta-manifest.json not found on the latest release");
        }
        const manifest = await fetchAssetJSON<DeltaManifest>(asset);
        return json(rewriteDeltaManifest(manifest, release.tag_name, url.origin));
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

console.log(`wailsrel proxy listening on http://${server.hostname}:${server.port}`);
