import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  output: 'standalone',
  typescript: {
    // !! WARN !!
    // Dangerously allow production builds to successfully complete even if
    // your project has type errors.
    ignoreBuildErrors: true,
  },
  async redirects() {
    return [
      {
        source: '/terms',
        destination: 'https://brokle.com/terms',
        permanent: false,
      },
      {
        source: '/privacy',
        destination: 'https://brokle.com/privacy',
        permanent: false,
      },
      {
        source: '/login',
        destination: '/signin',
        permanent: false,
      },
    ];
  },
};

export default nextConfig;
