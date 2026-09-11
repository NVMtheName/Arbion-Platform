"use client";

import { MotionConfig } from "motion/react";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

export function MotionProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return (
    <MotionConfig
      reducedMotion="user"
      transition={{
        duration: pathname === "/" ? 0.48 : 0.18,
        ease: [0.22, 1, 0.36, 1],
      }}
    >
      {children}
    </MotionConfig>
  );
}
