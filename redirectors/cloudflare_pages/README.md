# Cloudflare Pages 302 Redirector

Deploy this folder to Cloudflare Pages for a permanent streaming redirect domain.

### Deployment:
1. Connect this folder to Cloudflare Pages.
2. In Settings -> Environment Variables, add:
   - `TARGET_FQDN`: `your-backend-app.herokuapp.com`
3. Bind your custom domain in the Pages Custom Domains tab.
