const repository = requiredEnv("GITHUB_REPOSITORY");
const githubToken = optionalEnv("GITHUB_TOKEN");
const apiBaseUrl = optionalEnv("GITHUB_API_BASE_URL", "https://api.github.com").replace(/\/$/, "");
const host = optionalEnv("HOST", "127.0.0.1");
const port = Number(optionalEnv("PORT", "8788"));
const cacheTtlMs = Number(optionalEnv("CACHE_TTL_MS", "60000"));

const cache = {
  latest: undefined,
  byTag: new Map(),
};

function requiredEnv(name) {
  const value = Bun.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function optionalEnv(name, fallback = "") {
  return Bun.env[name]?.trim() || fallback;
}

function responseJSON(body, status = 200) {
  return new Response(`${JSON.stringify(body, null, 2)}\n`, {
    status,
    headers: { "content-type": "application/json" },
  });
}

function responseText(body, status = 200) {
  return new Response(body, {
    status,
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
}

function notFound(message = "not found") {
  return responseJSON({ error: message }, 404);
}

function badGateway(message = "bad gateway") {
  return responseJSON({ error: message }, 502);
}

function githubHeaders(accept, request) {
  const headers = {
    accept,
    "user-agent": "wailsrel-minimal-release-server",
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

async function fetchGitHubJSON(url) {
  const response = await fetch(url, {
    headers: githubHeaders("application/vnd.github+json"),
  });
  if (!response.ok) {
    throw new Error(`github request failed: ${response.status}`);
  }
  return response.json();
}

async function fetchGitHubAssetText(asset) {
  const response = await fetch(asset.url, {
    headers: githubHeaders("application/octet-stream"),
    redirect: "follow",
  });
  if (!response.ok) {
    throw new Error(`asset fetch failed: ${response.status}`);
  }
  return response.text();
}

async function latestRelease() {
  const now = Date.now();
  if (cache.latest && cache.latest.expiresAt > now) {
    return cache.latest.value;
  }
  const release = await fetchGitHubJSON(`${apiBaseUrl}/repos/${repository}/releases/latest`);
  cache.latest = { value: release, expiresAt: now + cacheTtlMs };
  cache.byTag.set(release.tag_name, { value: release, expiresAt: now + cacheTtlMs });
  return release;
}

async function releaseByTag(tag) {
  const now = Date.now();
  const cached = cache.byTag.get(tag);
  if (cached && cached.expiresAt > now) {
    return cached.value;
  }
  const release = await fetchGitHubJSON(`${apiBaseUrl}/repos/${repository}/releases/tags/${encodeURIComponent(tag)}`);
  cache.byTag.set(tag, { value: release, expiresAt: now + cacheTtlMs });
  return release;
}

function findAsset(release, name) {
  return release.assets.find((asset) => asset.name === name);
}

async function loadJSONAsset(release, name) {
  const asset = findAsset(release, name);
  if (!asset) {
    return undefined;
  }
  const text = await fetchGitHubAssetText(asset);
  return JSON.parse(text);
}

function proxyURL(origin, tag, assetName) {
  return `${origin}/download/${encodeURIComponent(tag)}/${encodeURIComponent(assetName)}`;
}

function assetNameFromValue(value) {
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

function rewriteReleaseManifest(manifest, tag, origin) {
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
    copy.delta.manifest_url = `${origin}/delta/manifest`;
  }

  for (const patch of copy.patches || []) {
    const assetName = assetNameFromValue(patch.url);
    if (assetName) {
      patch.url = proxyURL(origin, tag, assetName);
    }
  }

  for (const bundle of copy.frontend_bundles || []) {
    const assetName = assetNameFromValue(bundle.url);
    if (assetName) {
      bundle.url = proxyURL(origin, tag, assetName);
    }
  }

  return copy;
}

function rewriteDeltaManifest(manifest, tag, origin) {
  const copy = structuredClone(manifest);
  for (const patch of copy.patches || []) {
    const assetName = assetNameFromValue(patch.patch);
    if (assetName) {
      patch.patch = proxyURL(origin, tag, assetName);
    }
  }
  return copy;
}

async function streamAsset(request, asset) {
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

function infoPage() {
  return responseText(
    [
      "wailsrel minimal release server",
      "",
      "This example supports native update clients.",
      "",
      "Routes:",
      "  GET /manifest",
      "  GET /delta/manifest",
      "  GET /download/:tag/:asset_name",
      "  GET /healthz",
      "",
      "Notes:",
      "  - JSON only",
      "  - frontend catalog is not implemented",
      "  - GITHUB_TOKEN is optional for public repositories",
    ].join("\n"),
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
        return responseJSON({ ok: true, repository, cacheTtlMs });
      }

      if (url.pathname === "/manifest") {
        const release = await latestRelease();
        const manifest = await loadJSONAsset(release, "manifest.json");
        if (!manifest) {
          return notFound("manifest.json not found on the latest release");
        }
        return responseJSON(rewriteReleaseManifest(manifest, release.tag_name, url.origin));
      }

      if (url.pathname === "/delta/manifest") {
        const release = await latestRelease();
        const manifest = await loadJSONAsset(release, "delta-manifest.json");
        if (!manifest) {
          return notFound("delta-manifest.json not found on the latest release");
        }
        return responseJSON(rewriteDeltaManifest(manifest, release.tag_name, url.origin));
      }

      if (url.pathname === "/frontend/catalog") {
        return notFound("frontend catalog is not implemented in this example");
      }

      const match = url.pathname.match(/^\/download\/([^/]+)\/([^/]+)$/);
      if (match) {
        const tag = decodeURIComponent(match[1]);
        const assetName = decodeURIComponent(match[2]);
        const release = await releaseByTag(tag);
        const asset = findAsset(release, assetName);
        if (!asset) {
          return notFound(`asset ${assetName} not found on release ${tag}`);
        }
        return streamAsset(request, asset);
      }

      return notFound();
    } catch (error) {
      return badGateway(error instanceof Error ? error.message : "unexpected error");
    }
  },
});

console.log(`minimal release server listening on http://${server.hostname}:${server.port}`);
