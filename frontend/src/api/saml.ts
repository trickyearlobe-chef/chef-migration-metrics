// SPDX-License-Identifier: Apache-2.0

import { ApiError, apiFetch, buildUrl } from "./client";

export interface SAMLCertificateResponse {
  certificate_pem: string;
  fingerprint_sha256: string;
  not_after: string;
  subject?: string;
}

export function generateSAMLKeypair(): Promise<SAMLCertificateResponse> {
  return apiFetch<SAMLCertificateResponse>(buildUrl("/admin/saml/generate-keypair"), {
    method: "POST",
  });
}

export function fetchSAMLCertificate(): Promise<SAMLCertificateResponse | null> {
  return apiFetch<SAMLCertificateResponse>(buildUrl("/admin/saml/sp-certificate")).catch(
    (err: unknown) => {
      if (err instanceof ApiError && err.status === 404) return null;
      throw err;
    },
  );
}

// SAMLEndpoints holds the SP endpoint URLs an administrator must give the IdP.
// They are computed by the backend from the same base URL the SP metadata
// advertises, so a hand-configured IdP (Google, Okta) gets the exact ACS/reply
// URL rather than a guessed path.
export interface SAMLEndpoints {
  acs_url: string;
  slo_url: string;
  metadata_url: string;
  entity_id: string;
}

// fetchSAMLEndpoints returns the backend-computed SP endpoint URLs, or null when
// SAML is not configured (the endpoint returns 501 — no SP to describe).
export function fetchSAMLEndpoints(): Promise<SAMLEndpoints | null> {
  return apiFetch<SAMLEndpoints>(buildUrl("/admin/saml/endpoints")).catch((err: unknown) => {
    if (err instanceof ApiError && err.status === 501) return null;
    throw err;
  });
}

// samlMetadataUrl returns the absolute, externally-reachable URL of the SP
// metadata endpoint. IdPs that support metadata-by-URL (ADFS, Shibboleth,
// Keycloak, PingFederate, …) can be pointed at this to auto-refresh; others
// (Google, Okta) take the downloaded file instead. The endpoint is public.
export function samlMetadataUrl(): string {
  return `${window.location.origin}${buildUrl("/auth/saml/metadata")}`;
}

// fetchSAMLMetadata returns the live SP metadata document as raw XML. The
// endpoint serves application/samlmetadata+xml (not JSON), so we read the body
// as text rather than going through apiFetch. Returns 501 when no SAML provider
// is configured.
export async function fetchSAMLMetadata(): Promise<string> {
  const res = await fetch(buildUrl("/auth/saml/metadata"), {
    headers: { Accept: "application/samlmetadata+xml" },
  });
  if (res.ok) return res.text();

  let message = res.statusText || `HTTP ${res.status}`;
  const body = await res.text();
  try {
    const parsed = JSON.parse(body);
    if (parsed && typeof parsed === "object") {
      message = parsed.message || parsed.error || message;
    }
  } catch {
    if (body) message = body;
  }
  throw new ApiError(res.status, message, body);
}
