import { createHmac } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";
import type { BrowserContext } from "@playwright/test";

function base64url(input: Buffer | string): string { return Buffer.from(input).toString("base64url"); }
export function signSessionCookie(secret: string, claims: Record<string, unknown>): string {
  const header = base64url(JSON.stringify({ alg: "HS256", typ: "JWT" }));
  const payload = base64url(JSON.stringify(claims));
  const input = `${header}.${payload}`;
  return `${input}.${createHmac("sha256", secret).update(input).digest("base64url")}`;
}

export type IdentityState = "member" | "admin-locked" | "admin-elevated";
export interface FixtureIdentity { id: string; email: string; }
export interface E2EConfig { baseURL: string; jwtSecret: string; member: FixtureIdentity; admin: FixtureIdentity; }

function loadEnvFile(): Record<string, string> {
  const file = path.resolve(process.cwd(), "../.env.dev");
  const env: Record<string, string> = {};
  for (const raw of readFileSync(file, "utf8").split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#") || !line.includes("=")) continue;
    const [key, ...rest] = line.split("="); env[key] = rest.join("=");
  }
  return env;
}

function loadUsers(): Array<{ id: string; email: string; disabled?: boolean }> {
  return JSON.parse(readFileSync(path.resolve(process.cwd(), "../dev/data/pool_users.json"), "utf8"));
}

export function e2eConfig(): E2EConfig {
  const env = loadEnvFile();
  const users = loadUsers().filter(user => !user.disabled && user.email !== "developer@localhost.invalid");
  const adminEmails = new Set((env.DEV_ADMIN_EMAILS ?? "").toLowerCase().split(",").map(value => value.trim()).filter(Boolean));
  const admin = users.find(user => adminEmails.has(user.email.toLowerCase()));
  const member = users.find(user => !adminEmails.has(user.email.toLowerCase()));
  if (!env.DEV_POOL_JWT_SECRET || !admin || !member) throw new Error("E2E requires DEV_POOL_JWT_SECRET plus distinct enabled member and Admin fixture users");
  return { baseURL: process.env.POOL_BASE_URL ?? "http://127.0.0.1:18991", jwtSecret: env.DEV_POOL_JWT_SECRET, member, admin };
}

function token(secret: string, typ: "session" | "admin", user: FixtureIdentity): string {
  const now = Math.floor(Date.now() / 1000);
  return signSessionCookie(secret, { typ, sub: user.id, ...(typ === "session" ? { email: user.email } : {}), iat: now, exp: now + 3600 });
}

export async function authenticate(context: BrowserContext, state: IdentityState): Promise<void> {
  const cfg = e2eConfig();
  const user = state === "member" ? cfg.member : cfg.admin;
  const hostname = new URL(cfg.baseURL).hostname;
  const cookies = [{ name: "pool_session", value: token(cfg.jwtSecret, "session", user), domain: hostname, path: "/", httpOnly: true, secure: false, sameSite: "Lax" as const }];
  if (state === "admin-elevated") cookies.push({ name: "admin_elevated", value: token(cfg.jwtSecret, "admin", user), domain: hostname, path: "/", httpOnly: true, secure: false, sameSite: "Lax" as const });
  await context.addCookies(cookies);
}
