// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type {
  Program,
  ProgramMember,
  ProgramMentee,
  ProgramSponsor,
  ProgramTerm,
  TermStatus,
} from '../../app/types/program.types';
import { toProgramCardStatus, withActiveTerms } from '../../app/utils/program-terms';

export interface ProgramCatalogTerm {
  id: string;
  program_id: string;
  name: string;
  status: string;
  start_date_time?: string;
  end_date_time?: string;
  application_start_date?: string;
  application_end_date?: string;
  discovery_label?: string;
}

export interface ProgramCatalogMentor {
  id: string;
  user_id: string;
  name?: string;
  avatar_url?: string;
  introduction?: string;
}

export interface ProgramCatalogItem {
  id: string;
  name: string;
  slug: string;
  status: string;
  is_paid?: boolean;
  description?: string;
  logo_url?: string;
  repo_link?: string;
  cii_project_id?: string;
  code_of_conduct?: string;
  updated_on: string;
  skills?: string[];
  terms?: ProgramCatalogTerm[];
  mentors?: ProgramCatalogMentor[];
}

export interface ProgramCatalogMentee {
  user_id: string;
  name?: string;
  avatar_url?: string;
  introduction?: string;
  status: string;
  term_id: string;
  term_name: string;
}

export interface ProgramCatalogMenteesResponse {
  data: ProgramCatalogMentee[];
}

export interface ProgramCatalogSponsor {
  id: string;
  name: string;
  logo_url?: string;
  amount_cents: number;
}

export interface ProgramCatalogSponsorsResponse {
  data: ProgramCatalogSponsor[];
}

export interface ProgramCatalogListResponse {
  data: ProgramCatalogItem[];
  meta: { total: number; limit: number; offset: number };
}

const EMPTY_FOUNDATION = { id: '', name: '', slug: '' };

function toDate(iso?: string): string {
  if (!iso) return '';
  return iso.slice(0, 10);
}

function mapTerm(term: ProgramCatalogTerm): ProgramTerm {
  const status: TermStatus =
    term.status === 'closed' || term.status === 'deleted' ? term.status : 'open';
  return {
    id: term.id,
    name: term.name,
    status,
    startsAt: toDate(term.start_date_time),
    endsAt: toDate(term.end_date_time),
    applicationsStartAt: term.application_start_date,
    applicationsCloseAt: term.application_end_date,
  };
}

function mapMentor(mentor: ProgramCatalogMentor): ProgramMember {
  return {
    id: mentor.user_id,
    name: mentor.name?.trim() || 'Mentor',
    avatarUrl: mentor.avatar_url,
    intro: mentor.introduction?.trim() || undefined,
  };
}

function toMenteeStatus(status: string): ProgramMentee['status'] {
  return status === 'graduated' ? 'graduated' : 'active';
}

export function mapCatalogSponsor(sponsor: ProgramCatalogSponsor): ProgramSponsor {
  return {
    id: sponsor.id,
    name: sponsor.name?.trim() || 'Sponsor',
    logoUrl: sponsor.logo_url,
    amountCents: sponsor.amount_cents ?? 0,
  };
}

export function mapCatalogMentee(mentee: ProgramCatalogMentee): ProgramMentee {
  return {
    id: mentee.user_id,
    name: mentee.name?.trim() || 'Mentee',
    avatarUrl: mentee.avatar_url,
    intro: mentee.introduction?.trim() || undefined,
    status: toMenteeStatus(mentee.status),
    termId: mentee.term_id,
    termLabel: mentee.term_name,
  };
}

async function resolveProgramUID(id: string): Promise<string> {
  const config = useRuntimeConfig();
  const resolved = await $fetch<{ id: string }>(
    `${config.apiBaseUrl}/v1/programs/resolve/${encodeURIComponent(id)}`,
  );
  return resolved.id;
}

function fetchErrorStatus(error: unknown): number {
  if (typeof error === 'object' && error !== null && 'statusCode' in error) {
    const statusCode = Number((error as { statusCode?: number }).statusCode);
    if (Number.isFinite(statusCode) && statusCode > 0) return statusCode;
  }
  return 502;
}

export function mapCatalogItemToProgram(item: ProgramCatalogItem): Program {
  const terms = (item.terms ?? []).map(mapTerm);
  return withActiveTerms({
    id: item.id,
    slug: item.slug,
    name: item.name,
    description: item.description ?? '',
    logoUrl: item.logo_url,
    skills: item.skills ?? [],
    status: toProgramCardStatus(terms),
    foundation: EMPTY_FOUNDATION,
    terms,
    updatedAt: item.updated_on,
    repositoryUrl: item.repo_link,
    ciiProjectId: item.cii_project_id || undefined,
    codeOfConduct: item.code_of_conduct || undefined,
    mentees: [],
    mentors: (item.mentors ?? []).map(mapMentor),
    sponsors: [],
    isPaid: item.is_paid,
  });
}

export async function fetchProgramCatalog(query: {
  search?: string;
  skill?: string;
  status?: string;
  sortBy?: string;
  limit?: number;
  offset?: number;
}): Promise<ProgramCatalogListResponse> {
  const config = useRuntimeConfig();
  try {
    return await $fetch<ProgramCatalogListResponse>(`${config.apiBaseUrl}/v1/programs/catalog`, {
      query: {
        search: query.search || undefined,
        skill: query.skill && query.skill !== 'all' ? query.skill : undefined,
        status: query.status && query.status !== 'all' ? query.status : undefined,
        sortBy: query.sortBy || undefined,
        limit: query.limit ?? 15,
        offset: query.offset ?? 0,
      },
    });
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: 'Failed to load programs',
    });
  }
}

export async function fetchProgramCatalogItem(id: string): Promise<ProgramCatalogItem> {
  const config = useRuntimeConfig();
  try {
    const programUID = await resolveProgramUID(id);
    return await $fetch<ProgramCatalogItem>(
      `${config.apiBaseUrl}/v1/programs/${programUID}/catalog`,
    );
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: fetchErrorStatus(error) === 404 ? 'Program not found' : 'Failed to load program',
    });
  }
}

export async function fetchProgramMentees(id: string): Promise<ProgramCatalogMenteesResponse> {
  const config = useRuntimeConfig();
  try {
    const programUID = await resolveProgramUID(id);
    return await $fetch<ProgramCatalogMenteesResponse>(
      `${config.apiBaseUrl}/v1/programs/${programUID}/mentees`,
    );
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: fetchErrorStatus(error) === 404 ? 'Program not found' : 'Failed to load mentees',
    });
  }
}

export async function fetchProgramSponsors(id: string): Promise<ProgramCatalogSponsorsResponse> {
  const config = useRuntimeConfig();
  try {
    const programUID = await resolveProgramUID(id);
    return await $fetch<ProgramCatalogSponsorsResponse>(
      `${config.apiBaseUrl}/v1/programs/${programUID}/sponsors`,
      {
        query: {
          categoryType: 'mentorship',
          aggregate: true,
        },
      },
    );
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: fetchErrorStatus(error) === 404 ? 'Program not found' : 'Failed to load sponsors',
    });
  }
}
