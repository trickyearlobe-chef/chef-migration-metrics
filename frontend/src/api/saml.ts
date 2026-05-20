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
