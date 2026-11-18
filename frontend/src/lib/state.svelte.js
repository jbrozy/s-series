export const session = $state({ user: null, ready: false });

export const toasts = $state({ items: [] });
let nextId = 1;

export function toast(message, kind = 'ok', ms = 4000) {
  const id = nextId++;
  toasts.items.push({ id, message, kind });
  setTimeout(() => {
    const i = toasts.items.findIndex((t) => t.id === id);
    if (i >= 0) toasts.items.splice(i, 1);
  }, ms);
}

export function toastError(e) {
  toast(e?.message || String(e), 'error', 6000);
}
