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

interface OsanoConsentMap {
  ANALYTICS?: string;
  analytics?: string;
}

interface OsanoConsentManager {
  analytics?: boolean;
  getConsent?: () => OsanoConsentMap | undefined;
}

interface AnalyticsWindow {
  dataLayer?: Record<string, unknown>[];
  Osano?: { cm?: OsanoConsentManager };
  Intercom?: (...args: unknown[]) => void;
}

function analyticsWindow(): AnalyticsWindow | undefined {
  if (typeof window === 'undefined') return undefined;
  return window as unknown as AnalyticsWindow;
}

const firedOnce = new Set<string>();
const OSANO_RETRY_MS = 200;
const OSANO_RETRY_LIMIT = 10;

function hasOsano(): boolean {
  return Boolean(analyticsWindow()?.Osano?.cm);
}

function hasAnalyticsConsent(): boolean {
  const cm = analyticsWindow()?.Osano?.cm;
  if (!cm) {
    return useRuntimeConfig().public.appEnv !== 'production';
  }
  if (cm.analytics === true) return true;
  const consent = cm.getConsent?.();
  const value = consent?.ANALYTICS ?? consent?.analytics;
  return value === 'ACCEPT' || value === 'ACCEPT_ALL';
}

function emit(event: FunnelEventName, properties: FunnelProperties): void {
  const w = analyticsWindow();
  if (!w) return;
  const payload = { event, ...properties };
  w.dataLayer = w.dataLayer ?? [];
  w.dataLayer.push(payload);
  if (typeof w.Intercom === 'function') {
    try {
      w.Intercom('trackEvent', event, properties);
    } catch {
      // Intercom is optional — dataLayer is the source of truth for the funnel.
    }
  }
}

function tryTrack(
  event: FunnelEventName,
  properties: FunnelProperties,
  onceKey: string | undefined,
  attempt: number,
): void {
  if (!hasOsano() && attempt < OSANO_RETRY_LIMIT) {
    window.setTimeout(() => tryTrack(event, properties, onceKey, attempt + 1), OSANO_RETRY_MS);
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
