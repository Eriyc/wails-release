type GatewayConfig = {
  addr: string;
  repository: string;
  token: string;
  apiBaseUrl: string;
  jwksUrl: string;
  issuer: string;
  audience: string;
  cacheTtlMs: number;
};

type GitHubAsset = {
  id: number;
  name: string;
  url: string;
  content_type?: string;
  browser_download_url?: string;
};

type GitHubRelease = {
  id: number;
  tag_name: string;
  assets: GitHubAsset[];
};

type CachedValue<T> = {
  expiresAt: number;
  value: T;
};

type JwtHeader = {
  alg: string;
  kid?: string;
  typ?: string;
};

type JwtClaims = {
  iss?: string;
  aud?: string | string[];
  exp?: number;
  nbf?: number;
};

function requiredEnv(name: string): string {
  const value = Bun.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function loadConfig(): GatewayConfig {
  return {
    addr: Bun.env.WAILSREL_GATEWAY_ADDR?.trim() || ":8080",
    repository: requiredEnv("WAILSREL_GATEWAY_GITHUB_REPOSITORY"),
    token: requiredEnv("WAILSREL_GATEWAY_GITHUB_TOKEN"),
    apiBaseUrl: Bun.env.WAILSREL_GATEWAY_GITHUB_API_BASE_URL?.trim() || "https://api.github.com",
    jwksUrl: requiredEnv("WAILSREL_GATEWAY_JWKS_URL"),
    issuer: requiredEnv("WAILSREL_GATEWAY_JWT_ISSUER"),
    audience: requiredEnv("WAILSREL_GATEWAY_JWT_AUDIENCE"),
    cacheTtlMs: 60_000,
  };
}

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    ...init,
    headers: {
      "content-type": "application/json",
      ...init?.headers,
    },
  });
}

function unauthorized(message = "unauthorized"): Response {
  return jsonResponse({ error: message }, { status: 401 });
}

function notFound(message = "not found"): Response {
  return jsonResponse({ error: message }, { status: 404 });
}

function badGateway(message = "bad gateway"): Response {
  return jsonResponse({ error: message }, { status: 502 });
}

function parseAddr(addr: string): { hostname: string; port: number } {
  if (addr.startsWith(":")) {
    return { hostname: "0.0.0.0", port: Number(addr.slice(1)) };
  }
  const parsed = new URL(addr.includes("://") ? addr : `http://${addr}`);
  return {
    hostname: parsed.hostname || "0.0.0.0",
    port: Number(parsed.port || "8080"),
  };
}

class JwksVerifier {
  #jwksUrl: string;
  #issuer: string;
  #audience: string;
  #cache = new Map<string, CachedValue<JsonWebKey>>();

  constructor(config: GatewayConfig) {
    this.#jwksUrl = config.jwksUrl;
    this.#issuer = config.issuer;
    this.#audience = config.audience;
  }

  async verifyAuthorizationHeader(header: string | null): Promise<boolean> {
    if (!header || !header.startsWith("Bearer ")) {
      return false;
    }
    const token = header.slice("Bearer ".length).trim();
    if (!token) {
      return false;
    }
    return this.verifyToken(token);
  }

  async verifyToken(token: string): Promise<boolean> {
    const parts = token.split(".");
    if (parts.length !== 3) {
      return false;
    }

    const header = parseBase64UrlJSON<JwtHeader>(parts[0]);
    const claims = parseBase64UrlJSON<JwtClaims>(parts[1]);
    if (!header || !claims || header.alg !== "RS256" || !header.kid) {
      return false;
    }
    if (!validateClaims(claims, this.#issuer, this.#audience)) {
      return false;
    }

    const jwk = await this.#getKey(header.kid);
    if (!jwk) {
      return false;
    }

    const key = await crypto.subtle.importKey(
      "jwk",
      jwk,
      {
        name: "RSASSA-PKCS1-v1_5",
        hash: "SHA-256",
      },
      false,
      ["verify"],
    );

    const verified = await crypto.subtle.verify(
      "RSASSA-PKCS1-v1_5",
      key,
      decodeBase64Url(parts[2]),
      new TextEncoder().encode(`${parts[0]}.${parts[1]}`),
    );
    return verified;
  }

  async #getKey(kid: string): Promise<JsonWebKey | null> {
    const now = Date.now();
    const cached = this.#cache.get(kid);
    if (cached && cached.expiresAt > now) {
      return cached.value;
    }

    const response = await fetch(this.#jwksUrl, {
      headers: { accept: "application/json" },
    });
    if (!response.ok) {
      return null;
    }
    const body = (await response.json()) as { keys?: JsonWebKey[] };
    const keys = body.keys || [];
    const expiresAt = now + 5 * 60_000;
    for (const jwk of keys) {
      if (typeof jwk.kid === "string") {
        this.#cache.set(jwk.kid, { value: jwk, expiresAt });
      }
    }
    return this.#cache.get(kid)?.value || null;
  }
}

class GitHubReleaseCache {
  #config: GatewayConfig;
  #latest?: CachedValue<GitHubRelease>;
  #byTag = new Map<string, CachedValue<GitHubRelease>>();

  constructor(config: GatewayConfig) {
    this.#config = config;
  }

  async latest(): Promise<GitHubRelease> {
    const now = Date.now();
    if (this.#latest && this.#latest.expiresAt > now) {
      return this.#latest.value;
    }
    const release = await this.#fetchRelease(
      `${this.#config.apiBaseUrl}/repos/${this.#config.repository}/releases/latest`,
    );
    this.#latest = { value: release, expiresAt: now + this.#config.cacheTtlMs };
    this.#byTag.set(release.tag_name, { value: release, expiresAt: now + this.#config.cacheTtlMs });
    return release;
  }

  async byTag(tag: string): Promise<GitHubRelease> {
    const now = Date.now();
    const cached = this.#byTag.get(tag);
    if (cached && cached.expiresAt > now) {
      return cached.value;
    }
    const release = await this.#fetchRelease(
      `${this.#config.apiBaseUrl}/repos/${this.#config.repository}/releases/tags/${encodeURIComponent(tag)}`,
    );
    this.#byTag.set(tag, { value: release, expiresAt: now + this.#config.cacheTtlMs });
    return release;
  }

  async #fetchRelease(url: string): Promise<GitHubRelease> {
    const response = await fetch(url, {
      headers: githubHeaders(this.#config.token, "application/vnd.github+json"),
    });
    if (!response.ok) {
      throw new Error(`github release lookup failed: ${response.status}`);
    }
    return response.json() as Promise<GitHubRelease>;
  }
}

function githubHeaders(token: string, accept: string): HeadersInit {
  return {
    accept,
    authorization: `Bearer ${token}`,
    "user-agent": "wailsrel-bun-gateway",
  };
}

function findAsset(release: GitHubRelease, name: string): GitHubAsset | undefined {
  return release.assets.find((asset) => asset.name === name);
}

async function proxyGitHubAsset(config: GatewayConfig, asset: GitHubAsset): Promise<Response> {
  const response = await fetch(asset.url, {
    headers: githubHeaders(config.token, "application/octet-stream"),
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

export function createGateway(config = loadConfig()) {
  const verifier = new JwksVerifier(config);
  const releaseCache = new GitHubReleaseCache(config);

  return {
    async fetch(request: Request): Promise<Response> {
      const authorized = await verifier.verifyAuthorizationHeader(
        request.headers.get("authorization"),
      );
      if (!authorized) {
        return unauthorized();
      }

      const url = new URL(request.url);
      if (request.method !== "GET") {
        return notFound();
      }

      if (url.pathname === "/manifest.json") {
        try {
          const release = await releaseCache.latest();
          const asset = findAsset(release, "manifest.json");
          if (!asset) {
            return notFound("manifest.json not found");
          }
          return proxyGitHubAsset(config, asset);
        } catch (error) {
          return badGateway(String(error));
        }
      }

      if (url.pathname === "/delta/manifest.json") {
        try {
          const release = await releaseCache.latest();
          const asset = findAsset(release, "delta-manifest.json");
          if (!asset) {
            return notFound("delta-manifest.json not found");
          }
          return proxyGitHubAsset(config, asset);
        } catch (error) {
          return badGateway(String(error));
        }
      }

      const match = url.pathname.match(/^\/download\/([^/]+)\/([^/]+)$/);
      if (!match) {
        return notFound();
      }

      const [, encodedTag, encodedAssetName] = match;
      const tag = decodeURIComponent(encodedTag);
      const assetName = decodeURIComponent(encodedAssetName);

      try {
        const release = await releaseCache.byTag(tag);
        const asset = findAsset(release, assetName);
        if (!asset) {
          return notFound(`${assetName} not found`);
        }
        return proxyGitHubAsset(config, asset);
      } catch (error) {
        return badGateway(String(error));
      }
    },
  };
}

function parseBase64UrlJSON<T>(value: string): T | null {
  try {
    return JSON.parse(new TextDecoder().decode(decodeBase64Url(value))) as T;
  } catch {
    return null;
  }
}

function decodeBase64Url(value: string): Uint8Array {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - (normalized.length % 4 || 4)) % 4);
  const binary = atob(padded);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

function validateClaims(claims: JwtClaims, issuer: string, audience: string): boolean {
  const now = Math.floor(Date.now() / 1000);
  if (claims.iss !== issuer) {
    return false;
  }
  if (typeof claims.nbf === "number" && now < claims.nbf) {
    return false;
  }
  if (typeof claims.exp === "number" && now >= claims.exp) {
    return false;
  }

  if (typeof claims.aud === "string") {
    return claims.aud === audience;
  }
  if (Array.isArray(claims.aud)) {
    return claims.aud.includes(audience);
  }
  return false;
}

if (typeof Bun !== "undefined" && import.meta.main) {
  const config = loadConfig();
  const gateway = createGateway(config);
  const { hostname, port } = parseAddr(config.addr);
  Bun.serve({
    hostname,
    port,
    fetch: gateway.fetch,
  });
  console.log(`wailsrel Bun gateway listening on http://${hostname}:${port}`);
}
