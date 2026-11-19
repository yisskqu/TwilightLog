import createClient from "openapi-fetch";
import type { paths } from "./schema";

export const api = createClient<paths>({ baseUrl: "" });

export type { paths };

/** Display variants generated for every public photo. */
export type AssetVariant = "w480" | "w960" | "w1440" | "w2048" | "w2560";

export const ASSET_VARIANTS: AssetVariant[] = ["w480", "w960", "w1440", "w2048", "w2560"];

let assetBaseUrl = "/assets";

/**
 * Point the asset resolver at the current storage domain.
 * Local development uses "/assets"; production points at the CDN.
 */
export function configureAssetBaseUrl(url: string): void {
  assetBaseUrl = url.replace(/\/+$/, "");
}

/**
 * Build a public derivative URL. Never hard-code an image domain elsewhere.
 * `key` is the derivative prefix from the API, e.g. "media/<id>".
 */
export function assetUrl(key: string, variant?: AssetVariant): string {
  const clean = key.replace(/^\/+/, "").replace(/^public\//, "");
  if (!clean) {
    return assetBaseUrl;
  }
  if (variant && !clean.endsWith(".webp")) {
    return `${assetBaseUrl}/${clean}/${variant}.webp`;
  }
  return `${assetBaseUrl}/${clean}`;
}

/** Build a srcset string from the same resolver. */
export function assetSrcSet(key: string, variants: AssetVariant[] = ASSET_VARIANTS): string {
  return variants.map((variant) => `${assetUrl(key, variant)} ${variant.slice(1)}w`).join(", ");
}
