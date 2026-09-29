import { useEffect, useState } from "react";

export function useCountdown(deadline?: string): { label: string; elapsed: boolean } | null {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  if (!deadline) {
    return null;
  }
  const end = Date.parse(deadline);
  if (Number.isNaN(end)) {
    return null;
  }
  const remaining = end - now;
  const total = Math.max(0, Math.floor(remaining / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  const pad = (value: number) => String(value).padStart(2, "0");
  const label = hours > 0 ? `${hours}h ${pad(minutes)}m ${pad(seconds)}s` : `${minutes}m ${pad(seconds)}s`;
  return { label, elapsed: remaining <= 0 };
}
