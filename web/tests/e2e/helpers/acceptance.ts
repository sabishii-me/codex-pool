import { expect, test, type Page } from "@playwright/test";
import { authenticate, type IdentityState } from "./session";

export const canonicalRoutes = ["/", "/models", "/usage", "/setup", "/profile", "/admin/connections", "/admin/members", "/admin/system"] as const;

export function watchRuntime(page: Page) {
  const pageErrors: string[] = [];
  const consoleErrors: string[] = [];
  const requestFailures: string[] = [];
  const badResponses: string[] = [];
  page.on("pageerror", error => pageErrors.push(error.message));
  page.on("console", message => { if (message.type() === "error") consoleErrors.push(message.text()); });
  page.on("requestfailed", request => {
    // Reload intentionally aborts in-flight data refreshes from the document
    // being replaced. Those are browser lifecycle cancellations, not runtime failures.
    if (request.failure()?.errorText === "net::ERR_ABORTED") return;
    requestFailures.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? "failed"}`);
  });
  page.on("response", response => {
    const url = new URL(response.url());
    if (url.origin === new URL(page.url() || "http://127.0.0.1").origin && [429, 500, 502, 503, 504].includes(response.status())) badResponses.push(`${response.status()} ${url.pathname}`);
  });
  return { pageErrors, consoleErrors, requestFailures, badResponses };
}

export function expectClean(runtime: ReturnType<typeof watchRuntime>, options?: { allowSignedOutSession401?: boolean }) {
  expect(runtime.pageErrors, "page errors").toEqual([]);
  const consoleErrors = options?.allowSignedOutSession401
    ? runtime.consoleErrors.filter(message => !/401 \(Unauthorized\)/.test(message))
    : runtime.consoleErrors;
  expect(consoleErrors, "console errors").toEqual([]);
  expect(runtime.requestFailures, "failed requests").toEqual([]);
  expect(runtime.badResponses, "unexpected server responses").toEqual([]);
}

export async function loginAs(page: Page, state: IdentityState) { await authenticate(page.context(), state); }
