// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type {
  Program,
  ProgramMember,
  ProgramStatus,
  ProgramTerm,
  TermStatus,
} from '../../app/types/program.types';
import { getProgramTermDisplayStatus, withActiveTerms } from '../../app/utils/program-terms';

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
  updated_on: string;
  skills?: string[];
  terms?: ProgramCatalogTerm[];
  mentors?: ProgramCatalogMentor[];
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

function toUiStatus(terms: ProgramTerm[]): ProgramStatus {
  const displays = terms.map((term) => getProgramTermDisplayStatus(term));
  if (displays.some((status) => status === 'accepting')) return 'acceptance';
  if (displays.some((status) => status === 'opens-soon')) return 'open-soon';
  if (terms.some((term) => term.status === 'open')) return 'in-progress';
  return 'completed';
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
    status: toUiStatus(terms),
    foundation: EMPTY_FOUNDATION,
    terms,
    updatedAt: item.updated_on,
    repositoryUrl: item.repo_link,
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
    return await $fetch<ProgramCatalogItem>(`${config.apiBaseUrl}/v1/programs/${id}/catalog`);
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: fetchErrorStatus(error) === 404 ? 'Program not found' : 'Failed to load program',
    });
  }
}
