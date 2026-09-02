// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { computed, toValue, type MaybeRefOrGetter } from 'vue';
import { absoluteOgImage, SITE_NAME, siteOrigin, truncateMetaDescription } from '~/utils/seo';

export type PublicOgType = 'website' | 'profile' | 'article';

export interface PublicSeoOptions {
  title: MaybeRefOrGetter<string>;
  description: MaybeRefOrGetter<string>;
  /** Override the global canonical from `plugins/canonical.ts` (e.g. program slug). */
  path?: MaybeRefOrGetter<string>;
  /** Override the site-wide OG image (program logo only). */
  image?: MaybeRefOrGetter<string | undefined>;
  type?: MaybeRefOrGetter<PublicOgType>;
  noindex?: MaybeRefOrGetter<boolean>;
  /** When false, the document title is used as-is (home page). */
  appendSiteName?: boolean;
}

export function usePublicSeo(options: PublicSeoOptions) {
  const config = useRuntimeConfig();
  const origin = computed(() => siteOrigin(String(config.public.appUrl)));

  const title = computed(() => toValue(options.title));
  const description = computed(() => truncateMetaDescription(toValue(options.description), ''));
  const brandedTitle = computed(() => {
    if (options.appendSiteName === false || title.value === SITE_NAME) {
      return title.value;
    }
    return `${title.value} | ${SITE_NAME}`;
  });
  const canonical = computed(() => {
    const path = options.path !== undefined ? toValue(options.path) : undefined;
    if (!path) return undefined;
    const normalized = path.startsWith('/') ? path : `/${path}`;
    return `${origin.value}${normalized}`;
  });
  const image = computed(() => absoluteOgImage(toValue(options.image), origin.value));
  const type = computed(() => toValue(options.type) ?? 'website');
  const noindex = computed(() => Boolean(toValue(options.noindex)));

  useHead({
    title,
    titleTemplate: options.appendSiteName === false ? '%s' : `%s | ${SITE_NAME}`,
    link: computed(() => (canonical.value ? [{ rel: 'canonical', href: canonical.value }] : [])),
    meta: computed(() => (noindex.value ? [{ name: 'robots', content: 'noindex, nofollow' }] : [])),
  });

  useSeoMeta({
    description,
    ogTitle: brandedTitle,
    ogDescription: description,
    ogType: type,
    twitterCard: 'summary_large_image',
    twitterTitle: brandedTitle,
    twitterDescription: description,
    ...(options.path !== undefined ? { ogUrl: canonical } : {}),
    ...(options.image !== undefined ? { ogImage: image, twitterImage: image } : {}),
  });

  return { canonical, image, brandedTitle, origin };
}
