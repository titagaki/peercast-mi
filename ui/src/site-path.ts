// Vite's asset base and the Go site.base_path must describe the same mount point.
const base = import.meta.env.BASE_URL.replace(/\/$/, "");
export const siteURL = (path: string) => `${base}${path}`;
export const sitePath = () =>
  window.location.pathname.slice(base.length) || "/";
