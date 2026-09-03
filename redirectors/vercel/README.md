# Vercel Permanent 302 Redirector

Deploy this folder to Vercel to establish a permanent custom domain (e.g. `watch.mybrand.com`).

### Deployment:
1. Import this folder into Vercel.
2. In Project Settings -> Environment Variables, add:
   - `TARGET_FQDN`: `your-backend-app.herokuapp.com` (or your VPS IP / domain).
3. Connect your custom domain to this Vercel project.
4. Set `PERMANENT_REDIRECT_URL=https://watch.mybrand.com` in your backend `.env`.

Any request hitting `https://watch.mybrand.com/dl/<token>` or `/watch/<token>` will instantly 302 redirect to your active backend!
