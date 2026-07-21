import { createHmac } from "node:crypto";

function base64url(input: Buffer | string): string {
  return Buffer.from(input).toString("base64url");
}

// Mirrors signJWT() in pool_users.go exactly (HMAC-SHA256, header.payload.signature,
// all base64url) - lets tests mint a valid pool_session cookie without a browser
// OAuth round-trip, using the same POOL_JWT_SECRET the server verifies against.
export function signSessionCookie(secret: string, claims: Record<string, unknown>): string {
  const header = base64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const payload = base64url(JSON.stringify(claims));
  const signingInput = `${header}.${payload}`;
  const signature = createHmac("sha256", secret).update(signingInput).digest("base64url");
  return `${signingInput}.${signature}`;
}

export interface TestSessionConfig {
  jwtSecret: string;
  userID: string;
  email: string;
}

// Authenticated e2e tests need a real pool user already provisioned (the
// server resolves the session's `sub` against its pool_users.json) - read
// these from the environment rather than hardcoding a dev secret, and let
// callers skip cleanly when they're not configured.
export function readTestSessionConfig(): TestSessionConfig | null {
  const jwtSecret = process.env.POOL_JWT_SECRET;
  const userID = process.env.POOL_TEST_USER_ID;
  const email = process.env.POOL_TEST_USER_EMAIL;
  if (!jwtSecret || !userID || !email) return null;
  return { jwtSecret, userID, email };
}

export function sessionCookieValue(config: TestSessionConfig): string {
  const now = Math.floor(Date.now() / 1000);
  return signSessionCookie(config.jwtSecret, {
    typ: "session",
    sub: config.userID,
    email: config.email,
    iat: now,
    exp: now + 3600,
  });
}
