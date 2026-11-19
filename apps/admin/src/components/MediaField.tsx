import { useEffect, useRef, useState } from "react";
import { ApiError, apiRequest } from "../api";
import { mediaThumb, uploadImage, type MediaAsset } from "../media";

/** Uploads or previews one image and writes its media id into the payload. */
export function MediaField({
  value,
  purpose,
  onChange,
}: {
  value: string;
  purpose: "article" | "photo";
  onChange: (mediaId: string) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [asset, setAsset] = useState<MediaAsset | null>(null);
  const [busy, setBusy] = useState(false);
  const [label, setLabel] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    if (!value) {
      setAsset(null);
      return;
    }
    void apiRequest<MediaAsset>(`/api/v1/media/${value}`)
      .then((result) => {
        if (!cancelled) {
          setAsset(result);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setAsset(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [value]);

  async function handleFile() {
    const file = inputRef.current?.files?.[0];
    if (!file) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      const uploaded = await uploadImage(file, purpose, setLabel);
      setAsset(uploaded);
      onChange(uploaded.id);
    } catch (caught) {
      setError(caught instanceof ApiError || caught instanceof Error ? caught.message : "上传失败");
    } finally {
      setBusy(false);
      setLabel("");
      if (inputRef.current) {
        inputRef.current.value = "";
      }
    }
  }

  const thumb = mediaThumb(asset);
  return (
    <div className="media-field">
      <div className="media-thumb">
        {thumb ? <img src={thumb} alt="预览" /> : <span>{value ? "加载中…" : "未选择"}</span>}
      </div>
      <div className="media-field-body">
        <input
          ref={inputRef}
          type="file"
          accept={purpose === "photo" ? "image/jpeg,image/png,image/heic,image/heif" : "image/jpeg,image/png,image/webp"}
          onChange={() => void handleFile()}
          disabled={busy}
        />
        <div className="media-field-actions">
          {busy ? <span className="muted small-text">{label || "上传中…"}</span> : null}
          {value ? (
            <button type="button" className="btn small" onClick={() => onChange("")} disabled={busy}>
              清除
            </button>
          ) : null}
        </div>
        {value ? <code className="code">{value}</code> : null}
        {error ? <p className="alert small-text">{error}</p> : null}
      </div>
    </div>
  );
}
