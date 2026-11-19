import { useEffect, useState } from "react";
import type { PhotoSummary } from "../lib/api";
import PhotoViewer from "./PhotoViewer";

interface Props {
  photos: PhotoSummary[];
}

export default function AlbumPhotoViewer({ photos }: Props) {
  const [index, setIndex] = useState<number | null>(null);

  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      const tile = target?.closest<HTMLElement>("[data-photo-index]");
      if (!tile) return;
      const value = Number(tile.dataset.photoIndex);
      if (Number.isNaN(value)) return;
      event.preventDefault();
      setIndex(value);
    };
    document.addEventListener("click", onClick);
    return () => document.removeEventListener("click", onClick);
  }, []);

  return (
    <PhotoViewer
      photos={photos}
      index={index}
      onIndexChange={setIndex}
      onClose={() => setIndex(null)}
    />
  );
}
