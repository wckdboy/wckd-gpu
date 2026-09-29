import { useEffect, useState } from "react";

import { describeCountdown, type CountdownState } from "./format";

export function useCountdown(deadline?: string): CountdownState | null {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  if (!deadline) {
    return null;
  }
  return describeCountdown(deadline, now);
}
