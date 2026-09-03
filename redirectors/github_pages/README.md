# GitHub Pages Redirector

Host a free permanent redirect domain using GitHub Pages.

### Deployment:
1. Create a repository (e.g. `stream-redirect`) and push the files in this folder.
2. In Repository Settings -> Pages, select branch `main` and root `/`.
3. In Custom Domain, add your domain (e.g. `stream.yourdomain.com`).
4. Update `TARGET_FQDN` in `index.html` and `404.html`.
