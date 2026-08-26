import type { NextConfig } from "next";
import createNextIntlPlugin from 'next-intl/plugin';

// Point to the request configuration file
const withNextIntl = createNextIntlPlugin('./src/i18n/request.ts');

const isDev = process.env.NODE_ENV === "development";

const cspHeader = `
    default-src 'self';
    script-src 'self' 'unsafe-inline' ${isDev ? "'unsafe-eval'" : ''};
    style-src 'self' 'unsafe-inline';
    img-src 'self' blob: data:;
    font-src 'self';
    object-src 'none';
    base-uri 'self';
    form-action 'self';
    frame-ancestors 'none';
    connect-src 'self' ${isDev ? 'ws: wss:' : ''};
    ${isDev ? '' : 'upgrade-insecure-requests;'}
`;

const nextConfig: NextConfig = {
  reactCompiler: true,
  poweredByHeader: false,
  output: 'standalone',
  experimental: {
    globalNotFound: true,
  },
  images: { unoptimized: true },
  serverExternalPackages: ["@kubernetes/client-node"],
  turbopack: {
    root: __dirname,
  },
  async headers() {
    return [
      {
        source: '/(.*)',
        headers: [
          {
            key: 'Content-Security-Policy',
            value: cspHeader.replace(/\s{2,}/g, ' ').trim(),
          },
          { key: 'Referrer-Policy', value: 'no-referrer' },
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
          { key: 'Cross-Origin-Opener-Policy', value: 'same-origin' },
          ...(isDev ? [] : [{ key: 'Strict-Transport-Security', value: 'max-age=31536000; includeSubDomains' }]),
        ],
      },
    ];
  },
};

export default withNextIntl(nextConfig);
