import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/api/auth/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/auth/:path*`,
      },
      {
        source: "/api/admin/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/admin/:path*`,
      },
      {
        source: "/api/connections/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/connections/:path*`,
      },
    ];
  },
};

export default nextConfig;
