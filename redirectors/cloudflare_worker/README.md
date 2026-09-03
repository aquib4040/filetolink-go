# Cloudflare Worker 302 Redirector

Deploy this worker to handle ultra-fast edge 302 redirects from your permanent domain.

### Deployment:
1. Install Wrangler: `npm install -g wrangler`
2. Update `TARGET_FQDN` in `wrangler.toml` (or pass via Cloudflare dashboard).
3. Deploy: `wrangler deploy`
4. Add your custom domain route to this worker in Cloudflare dashboard (e.g. `stream.yourdomain.com/*`).
