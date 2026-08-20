import type { Metadata } from "next";
import type { ReactNode } from "react";

import { MotionProvider } from "./motion-provider";
import "./styles.css";

export const metadata: Metadata = {
  title: {
    default: "Arbion — Your financial command center",
    template: "%s · Arbion",
  },
  description:
    "A source-aware financial command center for disciplined, explainable decisions.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <MotionProvider>{children}</MotionProvider>
      </body>
    </html>
  );
}
