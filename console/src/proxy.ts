import createMiddleware from 'next-intl/middleware';
import { routing } from './i18n/routing';

const intlProxy = createMiddleware(routing);

export default intlProxy;

export const config = {
    matcher: ['/', '/en/:path*']
};
