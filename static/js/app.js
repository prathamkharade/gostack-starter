// App JavaScript. Rules (strict CSP, see docs/ui.md):
//  - No inline <script> or onclick in templates: put behaviour here.
//  - Alpine runs its CSP build: keep expressions in markup simple
//    (property reads, method names) and put logic in Alpine.data() below.
//  - htmx 4 event names use colons: htmx:after:request, htmx:config:request …

document.addEventListener("alpine:init", () => {
  // Mobile navigation. The menu uses the native `hidden` attribute (x-bind:hidden="closed"),
  // not x-show: x-show writes inline styles, which htmx copies while settling swaps and the CSP blocks.
  Alpine.data("navbar", () => ({
    open: false,
    get closed() { return !this.open; },
    toggle() { this.open = !this.open; },
    close() { this.open = false; },
  }));

  // Light/dark switch (initial value set by theme.js)
  Alpine.data("theme", () => ({
    toggle() {
      const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
      document.documentElement.dataset.theme = next;
      try { localStorage.setItem("theme", next); } catch (e) {}
    },
  }));

  // Toasts: the server sends HX-Trigger {"toast": {kind, msg}} (httpx.Toast)
  Alpine.data("toasts", () => ({
    items: [],
    seq: 0,
    add(event) {
      const d = (event && event.detail) || {};
      if (!d.msg) return;
      const id = ++this.seq;
      this.items.push({ id: id, msg: String(d.msg), cls: "alert-" + (d.kind || "info") });
      setTimeout(() => { this.items = this.items.filter((t) => t.id !== id); }, d.kind === "error" ? 8000 : 4000);
    },
  }));

  // Native <dialog> wrapper: x-data="dialog" + components.Dialog
  Alpine.data("dialog", () => ({
    show() { this.$refs.dialog.showModal(); },
    hide() { this.$refs.dialog.close(); },
  }));

  // Upload form: clear the file input after a successful upload
  Alpine.data("uploader", () => ({
    reset(event) {
      const ctx = event && event.detail && event.detail.ctx;
      if (!ctx || !ctx.response || ctx.response.status < 300) this.$root.reset();
    },
  }));
});

// Network errors and timeouts (no response from server) -> toast
document.addEventListener("htmx:error", () => {
  window.dispatchEvent(new CustomEvent("toast", { detail: { kind: "error", msg: "Network problem. Check your connection and try again." } }));
});

// Move focus to new content after boosted navigation (screen readers, keyboard users)
document.addEventListener("htmx:after:swap", (e) => {
  const ctx = e.detail && e.detail.ctx;
  if (ctx && ctx.target === document.body) {
    const main = document.getElementById("main");
    if (main) main.focus({ preventScroll: true });
  }
});
