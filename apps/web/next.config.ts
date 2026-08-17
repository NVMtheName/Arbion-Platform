import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
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
      {
        source: "/api/settings/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/settings/:path*`,
      },
      {
        source: "/api/neural/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/neural/:path*`,
      },
      {
        source: "/api/accounts/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/accounts/:path*`,
      },
      {
        source: "/api/automations/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/automations/:path*`,
      },
      {
        source: "/api/capital-buckets/:path*",
        destination: `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/capital-buckets/:path*`,
      },
    ];
  },
};

export default nextConfig;
