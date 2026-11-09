// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

/**
 * Public-site funnel events for issue #1518.
 *
 * Wizard events (`application_step_completed`, `application_submitted`,
 * `application_abandoned`) are exported for the apply flow; do not fire them
 * until that wizard exists. Never send PII or free-text answers as properties.
 */

export const FunnelEvent = {
  DirectoryViewed: 'directory_viewed',
  ProgramDetailViewed: 'program_detail_viewed',
  ApplyStarted: 'apply_started',
  ApplicationStepCompleted: 'application_step_completed',
  ApplicationSubmitted: 'application_submitted',
  ApplicationAbandoned: 'application_abandoned',
} as const;

export type FunnelEventName = (typeof FunnelEvent)[keyof typeof FunnelEvent];

export interface FunnelProperties {
  directory?: 'programs' | 'mentees' | 'mentors';
  program_id?: string;
  program_slug?: string;
  term_id?: string;
  step_name?: string;
}

// Transcend airgap.js (loaded in setup/head.ts).
interface TranscendAirgap {
  getConsent?: () => { purposes?: Record<string, boolean | string> } | undefined;
}

interface AnalyticsWindow {
  dataLayer?: Record<string, unknown>[];
  airgap?: TranscendAirgap;
}

function analyticsWindow(): AnalyticsWindow | undefined {
  if (typeof window === 'undefined') return undefined;
  return window as unknown as AnalyticsWindow;
}

const firedOnce = new Set<string>();
const CMP_RETRY_MS = 200;
const CMP_RETRY_LIMIT = 10;

function hasConsentManager(): boolean {
  return Boolean(analyticsWindow()?.airgap);
}

function hasAnalyticsConsent(): boolean {
  const airgap = analyticsWindow()?.airgap;
  if (!airgap) {
    return useRuntimeConfig().public.appEnv !== 'production';
  }
  return airgap.getConsent?.()?.purposes?.Analytics === true;
}

function emit(event: FunnelEventName, properties: FunnelProperties): void {
  const w = analyticsWindow();
  if (!w) return;
  const payload = { event, ...properties };
  w.dataLayer = w.dataLayer ?? [];
  w.dataLayer.push(payload);
}

function tryTrack(
  event: FunnelEventName,
  properties: FunnelProperties,
  onceKey: string | undefined,
  attempt: number,
): void {
  if (!hasConsentManager() && attempt < CMP_RETRY_LIMIT) {
    window.setTimeout(() => tryTrack(event, properties, onceKey, attempt + 1), CMP_RETRY_MS);
    return;
  }
  if (!hasAnalyticsConsent()) return;
  if (onceKey) {
    if (firedOnce.has(onceKey)) return;
    firedOnce.add(onceKey);
  }
  emit(event, properties);
}

export function trackFunnelEvent(
  event: FunnelEventName,
  properties: FunnelProperties = {},
  onceKey?: string,
): void {
  if (!import.meta.client) return;
  tryTrack(event, properties, onceKey, 0);
}

export function useFunnelPageView(
  event: FunnelEventName,
  properties: FunnelProperties,
  onceKey?: string,
): void {
  onMounted(() => {
    trackFunnelEvent(event, properties, onceKey ?? event);
  });
}
