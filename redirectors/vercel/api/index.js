// Vercel Edge 302 Redirect Handler
// Dynamically reads process.env.TARGET_FQDN set in Vercel Project Settings
export const config = {
  runtime: 'edge',
};

export default function handler(request) {
  const url = new URL(request.url);
  const target = (process.env.TARGET_FQDN || 'filetolinkcanonbots-4b978cf5a538.herokuapp.com')
    .replace(/^https?:\/\//, '')
    .replace(/\/+$/, '');

  const destination = `https://${target}${url.pathname}${url.search}`;
  return Response.redirect(destination, 302);
}
