// Cloudflare Pages Function: Dynamic 302 Redirect with TARGET_FQDN env variable
export async function onRequest(context) {
  const url = new URL(context.request.url);
  const target = (context.env.TARGET_FQDN || "filetolinkcanonbots-4b978cf5a538.herokuapp.com")
    .replace(/^https?:\/\//, "")
    .replace(/\/+$/, "");

  const destination = `https://${target}${url.pathname}${url.search}`;
  return Response.redirect(destination, 302);
}
