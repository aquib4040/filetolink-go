// Cloudflare Worker 302 Redirector
// Forwards all paths & query parameters to active streaming backend
export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const target = (env.TARGET_FQDN || "filetolinkcanonbots-4b978cf5a538.herokuapp.com")
      .replace(/^https?:\/\//, "")
      .replace(/\/+$/, "");

    const destination = `https://${target}${url.pathname}${url.search}`;
    return Response.redirect(destination, 302);
  }
};
