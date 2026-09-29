import { useEffect, useState } from "react";

import { catalogAccrual } from "./format";

export function useCostTicker(usdPerHr: number, startedAt: string | undefined, active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) {
      return;
    }
    const timer = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [active]);
  if (!startedAt) {
    return 0;
  }
  return catalogAccrual(usdPerHr, startedAt, now);
}
