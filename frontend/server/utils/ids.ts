// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUUID(value: string): boolean {
  return UUID.test(value);
}
