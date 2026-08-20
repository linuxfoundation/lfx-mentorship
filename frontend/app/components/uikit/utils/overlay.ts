// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

let scrollLockCount = 0;
let previousOverflow = '';

export function lockBodyScroll() {
  if (!import.meta.client) return;
  if (scrollLockCount === 0) {
    previousOverflow = document.documentElement.style.overflow;
    document.documentElement.style.overflow = 'hidden';
  }
  scrollLockCount += 1;
}

export function unlockBodyScroll() {
  if (!import.meta.client) return;
  scrollLockCount = Math.max(0, scrollLockCount - 1);
  if (scrollLockCount === 0) {
    document.documentElement.style.overflow = previousOverflow;
  }
}

export function getFocusableElements(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => el.getAttribute('aria-hidden') !== 'true',
  );
}

export function trapFocus(event: KeyboardEvent, root: HTMLElement) {
  if (event.key !== 'Tab') return;

  const nodes = getFocusableElements(root);
  if (nodes.length === 0) {
    event.preventDefault();
    root.focus();
    return;
  }

  const first = nodes[0];
  const last = nodes[nodes.length - 1];
  const active = document.activeElement;

  if (event.shiftKey && active === first) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && active === last) {
    event.preventDefault();
    first?.focus();
  }
}

export function focusFirst(root: HTMLElement) {
  const first = getFocusableElements(root)[0];
  (first ?? root).focus();
}
