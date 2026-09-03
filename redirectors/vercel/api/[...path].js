// Vercel Edge 302 Redirect Handler
// Supports TARGET_FQDN environment variable
export const config = {
  runtime: 'edge',
};

export default function handler(request) {
  const url = new URL(request.url);
  const target = (process.env.TARGET_FQDN || 'YOUR-STREAMING-FQDN.herokuapp.com')
    .replace(/^https?:\/\//, '')
    .replace(/\/+$/, '');

  const destination = `https://${target}${url.pathname}${url.search}`;
  return Response.redirect(destination, 302);
}
