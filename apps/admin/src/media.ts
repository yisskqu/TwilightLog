import { apiRequest } from "./api";

export type MediaAsset = {
  id: string;
  purpose: "article" | "photo";
  status: "uploading" | "processing" | "ready" | "failed";
  originalFilename: string;
  mimeType: string;
  fileSize: number;
  width: number;
  height: number;
  blurhash: string;
  variants: { name: string; url: string; width: number; height: number; size: number }[];
  error?: { code: string; message: string };
};

type PresignResponse = {
  mediaId: string;
  uploadMode: "single" | "multipart";
  uploadUrl?: string;
  partUrls?: string[];
  partSize?: number;
  headers: Record<string, string>;
};

const MIME_BY_EXT: Record<string, string> = {
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  webp: "image/webp",
  heic: "image/heic",
  heif: "image/heic",
};

function contentTypeOf(file: File): string {
  const type = (file.type || "").toLowerCase();
  if (type === "image/jpg") {
    return "image/jpeg";
  }
  if (type === "image/heif") {
    return "image/heic";
  }
  if (type) {
    return type;
  }
  const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
  return MIME_BY_EXT[ext] ?? "application/octet-stream";
}

async function putBlob(url: string, blob: Blob, contentType: string): Promise<void> {
  const response = await fetch(url, { method: "PUT", headers: { "Content-Type": contentType }, body: blob });
  if (!response.ok) {
    throw new Error(`上传失败（HTTP ${response.status}）`);
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

/** Uploads one file through the presign pipeline and waits for processing. */
export async function uploadImage(
  file: File,
  purpose: "article" | "photo",
  onProgress?: (label: string) => void,
): Promise<MediaAsset> {
  onProgress?.("申请上传地址");
  const batchId = crypto.randomUUID();
  const presign = await apiRequest<PresignResponse>("/api/v1/media/presign", {
    method: "POST",
    body: JSON.stringify({
      purpose,
      filename: file.name,
      contentType: contentTypeOf(file),
      size: file.size,
      batchId,
    }),
  });

  onProgress?.("上传中");
  if (presign.uploadMode === "multipart") {
    const partSize = presign.partSize ?? 5 * 1024 * 1024;
    const urls = presign.partUrls ?? [];
    for (let index = 0; index < urls.length; index += 1) {
      const start = index * partSize;
      const blob = file.slice(start, Math.min(start + partSize, file.size));
      await putBlob(urls[index], blob, "application/octet-stream");
      onProgress?.(`上传中 ${Math.round(((index + 1) / urls.length) * 100)}%`);
    }
  } else if (presign.uploadUrl) {
    await putBlob(presign.uploadUrl, file, presign.headers["Content-Type"] ?? contentTypeOf(file));
  } else {
    throw new Error("服务端未返回上传地址");
  }

  onProgress?.("处理图片");
  let asset = await apiRequest<MediaAsset>("/api/v1/media/complete", {
    method: "POST",
    body: JSON.stringify({ mediaId: presign.mediaId }),
  });
  const deadline = Date.now() + 120_000;
  while (asset.status === "processing" || asset.status === "uploading") {
    if (Date.now() > deadline) {
      throw new Error("图片处理超时");
    }
    await sleep(800);
    asset = await apiRequest<MediaAsset>(`/api/v1/media/${presign.mediaId}`);
  }
  if (asset.status === "failed") {
    throw new Error(asset.error?.message ?? "图片处理失败");
  }
  return asset;
}

export function mediaThumb(asset: MediaAsset | null): string {
  if (!asset) {
    return "";
  }
  return asset.variants.find((item) => item.name === "w480")?.url ?? asset.variants[0]?.url ?? "";
}
