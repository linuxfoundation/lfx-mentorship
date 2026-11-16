// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { expect, type Locator, type Page } from '@playwright/test';

// Helpers for the PrimeNG controls Self Serve's Mentorship forms are built from.

const SELECT_OVERLAY = '.p-select-overlay';

/** A select: the `data-test` attribute its wrapper carries, or its locator. */
export type SelectRef = string | Locator;

function selectLocator(page: Page, select: SelectRef): Locator {
  return typeof select === 'string' ? page.locator(`[data-test="${select}"]`) : select;
}

/**
 * Opens a select, or returns its overlay if it is already open. Clicks only once no other overlay
 * is left: a click while one is still animating closed can land on whatever is behind it.
 */
export async function openSelect(page: Page, ref: SelectRef): Promise<Locator> {
  const select = selectLocator(page, ref);
  const combobox = select.getByRole('combobox').first();
  const overlay = page.locator(SELECT_OVERLAY);
  await expect(async () => {
    if ((await combobox.getAttribute('aria-expanded')) !== 'true') {
      await expect(overlay).toHaveCount(0, { timeout: 3_000 });
      await select.click();
    }
    await expect(combobox).toHaveAttribute('aria-expanded', 'true', { timeout: 2_000 });
  }).toPass({ timeout: 15_000 });
  return overlay;
}

/** Picks the option with that exact label, typing `filter` into the select's search box first. */
export async function pickOption(
  page: Page,
  ref: SelectRef,
  label: string,
  filter?: string,
): Promise<void> {
  const overlay = await openSelect(page, ref);
  if (filter) await overlay.locator('input.p-select-filter').fill(filter);
  await overlay.getByRole('option', { name: label, exact: true }).click();
  const select = selectLocator(page, ref);
  await expect(select).toContainText(label);
  // The overlay closes by itself, but some pickers reopen while the form refills. Close it by
  // toggling the select: Escape would also close a dialog the select sits in.
  try {
    await expect(page.locator(SELECT_OVERLAY)).toHaveCount(0, { timeout: 5_000 });
  } catch {
    await select.click();
    await expect(page.locator(SELECT_OVERLAY)).toHaveCount(0);
  }
}

/**
 * Types a YYYY-MM-DD date into a date picker and leaves it with Tab, which closes its overlay
 * (Escape would also close an enclosing dialog). The picker ignores input without a keydown, so a
 * key press comes first and the value then goes in as one input.
 */
export async function fillDate(input: Locator, value: string): Promise<void> {
  await input.click();
  await input.press('End');
  await input.fill(value);
  await input.press('Tab');
  await expect(input).toHaveValue(value);
}

/** Adds a skill with a skills picker; `idPrefix` is the picker's, e.g. `mentorship-enroll`. */
export async function addSkill(page: Page, skill: string, idPrefix: string): Promise<void> {
  await page.locator(`#${idPrefix}-skill-input`).fill(skill);
  await page.keyboard.press('Escape');
  await page.getByTestId(`${idPrefix}-add-skill`).click();
  await expect(page.getByTestId(`${idPrefix}-skill-list`)).toContainText(skill);
}

/** Types into the one rich-text editor inside `scope`. */
export async function fillRichText(scope: Locator, text: string): Promise<void> {
  const editor = scope.locator('[contenteditable="true"]');
  await editor.click();
  await editor.fill(text);
}

/** YYYY-MM-DD in UTC, the backend's timezone; the browser runs in UTC too. */
export function isoDate(date: Date): string {
  return date.toISOString().slice(0, 10);
}
