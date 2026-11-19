import type { Asset, AssetVariant } from "./api";

const DATE = new Intl.DateTimeFormat("zh-CN", {
  timeZone: "Asia/Shanghai",
  year: "numeric",
  month: "long",
  day: "numeric",
});

const DATE_SHORT = new Intl.DateTimeFormat("zh-CN", {
  timeZone: "Asia/Shanghai",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

export function formatDate(value: string | null | undefined): string {
  const time = parseTime(value);
  return time ? DATE.format(time) : "";
}

export function formatDateShort(value: string | null | undefined): string {
  const time = parseTime(value);
  return time ? DATE_SHORT.format(time).replaceAll("/", ".") : "";
}

function parseTime(value: string | null | undefined): Date | null {
  if (!value) return null;
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? null : time;
}

export interface ImageSource {
  src: string;
  srcset: string;
  width: number;
  height: number;
}

const WIDTH_ORDER: AssetVariant["name"][] = ["w480", "w960", "w1440", "w2048", "w2560"];

/** Pick a display variant, falling back to the closest one the API returned. */
export function imageSource(asset: Asset | null | undefined, preferred = "w960"): ImageSource | null {
  if (!asset?.variants?.length) return null;
  const variants = [...asset.variants].sort(
    (a, b) => WIDTH_ORDER.indexOf(a.name) - WIDTH_ORDER.indexOf(b.name),
  );
  const preferredIndex = WIDTH_ORDER.indexOf(preferred);
  const chosen =
    variants.find((variant) => variant.name === preferred) ??
    variants.find((variant) => WIDTH_ORDER.indexOf(variant.name) >= preferredIndex) ??
    variants[variants.length - 1];
  return {
    src: chosen.url,
    srcset: variants.map((variant) => `${variant.url} ${variant.width}w`).join(", "),
    width: chosen.width,
    height: chosen.height,
  };
}

export function absoluteUrl(url: string, origin: string): string {
  if (!url) return "";
  try {
    return new URL(url, origin).toString();
  } catch {
    return url;
  }
}

export function readingTime(markdown: string): string {
  const chinese = (markdown.match(/[\u4e00-\u9fff]/g) ?? []).length;
  const words = markdown.replace(/[\u4e00-\u9fff]/g, " ").split(/\s+/).filter(Boolean).length;
  const minutes = Math.max(1, Math.ceil(chinese / 400 + words / 220));
  return `${minutes} 分钟阅读`;
}
