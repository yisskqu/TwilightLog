import { useRef, useState } from "react";
import { ApiError, apiRequest } from "../api";

type MediaVariant = {
  name: string;
  url: string;
  width: number;
  height: number;
  size: number;
};

type MediaExif = {
  cameraMake?: string;
  cameraModel?: string;
  lensModel?: string;
  focalLength?: string;
  aperture?: string;
  shutterSpeed?: string;
  iso?: number;
  takenAt?: string;
  timeZone?: string;
  exposureBias?: string;
  whiteBalance?: string;
  orientation?: number;
  sourceWidth?: number;
  sourceHeight?: number;
};

type MediaAsset = {
  id: string;
  purpose: "article" | "photo";
  status: "uploading" | "processing" | "ready" | "failed";
  originalFilename: string;
  mimeType: string;
  fileSize: number;
  width: number;
  height: number;
  blurhash: string;
  exif: MediaExif;
  variants: MediaVariant[];
  originalUrl?: string;
  error?: { code: string; message: string };
  processedAt?: string;
  createdAt: string;
  updatedAt: string;
};

type PresignResponse = {
  mediaId: string;
  status: string;
  uploadMode: "single" | "multipart";
  uploadUrl?: string;
  partUrls?: string[];
  partSize?: number;
  partCount?: number;
  headers: Record<string, string>;
  expiresIn: number;
};

type ItemStatus = "pending" | "presigning" | "uploading" | "processing" | "ready" | "failed";

type UploadItem = {
  id: string;
  file: File;
  status: ItemStatus;
  progress: number;
  error?: string;
  asset?: MediaAsset;
};

const MIME_BY_EXT: Record<string, string> = {
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  webp: "image/webp",
  heic: "image/heic",
  heif: "image/heic",
};

const statusText: Record<ItemStatus, string> = {
  pending: "等待上传",
  presigning: "申请地址",
  uploading: "上传中",
  processing: "处理中",
  ready: "已完成",
  failed: "失败",
};

function normalizedContentType(file: File): string {
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

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function putBlob(url: string, blob: Blob, contentType: string): Promise<void> {
  const response = await fetch(url, {
    method: "PUT",
    headers: { "Content-Type": contentType },
    body: blob,
  });
  if (!response.ok) {
    let message = `上传失败（HTTP ${response.status}）`;
    try {
      const body = (await response.json()) as { message?: string };
      message = body.message ?? message;
    } catch {
      // The upload endpoint may return an empty body.
    }
    throw new Error(message);
  }
}

function primaryVariant(asset: MediaAsset): MediaVariant | undefined {
  return asset.variants.find((variant) => variant.name === "w480") ?? asset.variants[0];
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function exifSummary(exif: MediaExif): string {
  const parts = [
    [exif.cameraMake, exif.cameraModel].filter(Boolean).join(" "),
    exif.lensModel,
    exif.focalLength,
    exif.aperture,
    exif.shutterSpeed,
    exif.iso ? `ISO ${exif.iso}` : "",
    exif.takenAt,
    exif.orientation ? `方向 ${exif.orientation}` : "",
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" · ") : "无 EXIF 信息";
}

function AssetCard({ asset }: { asset: MediaAsset }) {
  const variant = primaryVariant(asset);
  return (
    <article className="media-card">
      <div className="media-thumb">
        {variant ? <img src={variant.url} alt={asset.originalFilename} loading="lazy" /> : <span>无缩略图</span>}
      </div>
      <div className="media-body">
        <div className="media-head">
          <strong className="truncate" title={asset.originalFilename}>
            {asset.originalFilename}
          </strong>
          <span className={`badge status-${asset.status === "ready" ? "active" : asset.status === "failed" ? "disabled" : "pending"}`}>
            {asset.status === "ready" ? "可用" : asset.status === "failed" ? "失败" : asset.status}
          </span>
        </div>
        <p className="muted small-text">
          {asset.width && asset.height ? `${asset.width} × ${asset.height} · ` : ""}
          {formatBytes(asset.fileSize)} · {asset.mimeType}
        </p>
        {asset.blurhash ? <code className="code">blurhash: {asset.blurhash}</code> : null}
        {asset.status === "failed" && asset.error ? (
          <p className="alert small-text">
            {asset.error.code}: {asset.error.message}
          </p>
        ) : null}
        <details className="media-details">
          <summary className="small-text">查看变体与 EXIF</summary>
          <p className="muted small-text">{exifSummary(asset.exif)}</p>
          <ul className="variant-list">
            {asset.variants.map((item) => (
              <li key={item.name}>
                <a href={item.url} target="_blank" rel="noreferrer">
                  {item.name}
                </a>
                <span className="muted small-text">
                  {" "}
                  {item.width}×{item.height} · {formatBytes(item.size)}
                </span>
              </li>
            ))}
          </ul>
          {asset.originalUrl ? (
            <a className="small-text" href={asset.originalUrl} target="_blank" rel="noreferrer">
              打开签名原图（5 分钟有效）
            </a>
          ) : null}
          <code className="code">{asset.id}</code>
        </details>
      </div>
    </article>
  );
}

export function MediaTestPage() {
  const [purpose, setPurpose] = useState<"article" | "photo">("photo");
  const [items, setItems] = useState<UploadItem[]>([]);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [queryId, setQueryId] = useState("");
  const [queried, setQueried] = useState<MediaAsset | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  function patchItem(id: string, patch: Partial<UploadItem>) {
    setItems((previous) => previous.map((item) => (item.id === id ? { ...item, ...patch } : item)));
  }

  async function uploadOne(item: UploadItem, batchId: string): Promise<void> {
    const file = item.file;
    patchItem(item.id, { status: "presigning", progress: 0, error: undefined });
    const presign = await apiRequest<PresignResponse>("/api/v1/media/presign", {
      method: "POST",
      headers: { "Idempotency-Key": `${batchId}:${item.id}` },
      body: JSON.stringify({
        purpose,
        filename: file.name,
        contentType: normalizedContentType(file),
        size: file.size,
        batchId,
      }),
    });

    patchItem(item.id, { status: "uploading" });
    if (presign.uploadMode === "multipart") {
      const partSize = presign.partSize ?? 5 * 1024 * 1024;
      const urls = presign.partUrls ?? [];
      for (let index = 0; index < urls.length; index += 1) {
        const start = index * partSize;
        const blob = file.slice(start, Math.min(start + partSize, file.size));
        await putBlob(urls[index], blob, "application/octet-stream");
        patchItem(item.id, { progress: Math.round(((index + 1) / urls.length) * 100) });
      }
    } else if (presign.uploadUrl) {
      await putBlob(presign.uploadUrl, file, presign.headers["Content-Type"] ?? normalizedContentType(file));
      patchItem(item.id, { progress: 100 });
    } else {
      throw new Error("服务端未返回上传地址");
    }

    patchItem(item.id, { status: "processing" });
    let asset = await apiRequest<MediaAsset>("/api/v1/media/complete", {
      method: "POST",
      body: JSON.stringify({ mediaId: presign.mediaId }),
    });

    const deadline = Date.now() + 120_000;
    while (asset.status === "processing" || asset.status === "uploading") {
      if (Date.now() > deadline) {
        throw new Error("处理超时，请稍后用媒体 ID 查询");
      }
      await sleep(800);
      asset = await apiRequest<MediaAsset>(`/api/v1/media/${presign.mediaId}`);
      patchItem(item.id, { status: asset.status as ItemStatus, asset });
    }

    if (asset.status === "failed") {
      patchItem(item.id, {
        status: "failed",
        asset,
        error: asset.error ? `${asset.error.code}: ${asset.error.message}` : "处理失败",
      });
      return;
    }
    patchItem(item.id, { status: "ready", asset, progress: 100 });
  }

  async function handleUpload() {
    const files = Array.from(fileInputRef.current?.files ?? []);
    if (files.length === 0) {
      setMessage("请先选择图片");
      return;
    }
    if (files.length > 20) {
      setMessage("单次最多 20 张，请减少选择");
      return;
    }
    const batchId = crypto.randomUUID();
    const nextItems: UploadItem[] = files.map((file) => ({
      id: crypto.randomUUID(),
      file,
      status: "pending",
      progress: 0,
    }));
    setItems(nextItems);
    setMessage(`批次 ${batchId}，共 ${files.length} 张`);
    setBusy(true);
    for (const item of nextItems) {
      try {
        await uploadOne(item, batchId);
      } catch (caught) {
        const reason = caught instanceof ApiError || caught instanceof Error ? caught.message : "上传失败";
        patchItem(item.id, { status: "failed", error: reason });
      }
    }
    setBusy(false);
  }

  async function handleQuery() {
    const id = queryId.trim();
    if (!id) {
      return;
    }
    setMessage("");
    try {
      const asset = await apiRequest<MediaAsset>(`/api/v1/media/${id}`);
      setQueried(asset);
    } catch (caught) {
      setQueried(null);
      setMessage(caught instanceof ApiError ? caught.message : "查询失败");
    }
  }

  return (
    <section className="page">
      <header className="page-head">
        <div>
          <span className="eyebrow">媒体管线</span>
          <h1>上传测试</h1>
          <p className="subtitle">
            选择图片后直接上传到本地存储。文章图片支持 JPG/PNG/WebP（≤5MB），摄影照片支持 JPG/PNG/HEIC（≤50MB），单批最多 20 张。
          </p>
        </div>
      </header>

      <div className="create-panel">
        <label className="field">
          <span>用途</span>
          <select value={purpose} onChange={(event) => setPurpose(event.target.value as "article" | "photo")} disabled={busy}>
            <option value="photo">摄影照片</option>
            <option value="article">文章图片</option>
          </select>
        </label>
        <label className="field">
          <span>选择图片</span>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            accept={purpose === "photo" ? "image/jpeg,image/png,image/heic,image/heif" : "image/jpeg,image/png,image/webp"}
            disabled={busy}
          />
        </label>
        <button type="button" className="btn primary" onClick={() => void handleUpload()} disabled={busy}>
          {busy ? "上传中…" : "开始上传"}
        </button>
      </div>

      {message ? <p className="muted small-text">{message}</p> : null}

      {items.length > 0 ? (
        <div className="upload-list">
          {items.map((item) => (
            <div key={item.id} className="upload-row">
              <span className={`badge status-${item.status === "ready" ? "active" : item.status === "failed" ? "disabled" : "pending"}`}>
                {statusText[item.status]}
              </span>
              <span className="truncate" title={item.file.name}>
                {item.file.name}
              </span>
              <span className="muted small-text">{formatBytes(item.file.size)}</span>
              <span className="muted small-text">
                {item.status === "uploading" ? `${item.progress}%` : item.error ?? ""}
              </span>
            </div>
          ))}
        </div>
      ) : null}

      {items.some((item) => item.asset) ? (
        <div className="media-grid">
          {items.map((item) => (item.asset ? <AssetCard key={`asset-${item.id}`} asset={item.asset} /> : null))}
        </div>
      ) : null}

      <div className="panel">
        <h2>按媒体 ID 查询</h2>
        <div className="search">
          <input
            type="text"
            placeholder="粘贴 mediaId"
            value={queryId}
            onChange={(event) => setQueryId(event.target.value)}
          />
          <button type="button" className="btn" onClick={() => void handleQuery()}>
            查询
          </button>
        </div>
        {queried ? <div className="media-grid" style={{ marginTop: 20 }}>{<AssetCard asset={queried} />}</div> : null}
      </div>
    </section>
  );
}
