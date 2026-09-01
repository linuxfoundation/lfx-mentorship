// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { ProfileProgram, ProfileProgramTerm } from '../../app/types/mentee.types';
import type { ProgramTerm, TermStatus } from '../../app/types/program.types';
import { toProgramCardStatus } from '../../app/utils/program-terms';
import type {
  Mentor,
  MentorDetail,
  MentorMenteeSummary,
  MentorStats,
  MentorsListResponse,
  MentorsSummaryResponse,
} from '../../app/types/mentor.types';

export interface MentorApiMentor {
  id: string;
  user_id: string;
  name?: string;
  avatar_url?: string;
  introduction?: string;
}

export interface MentorProgramTerm {
  id: string;
  name: string;
  status?: string;
  start_date_time?: string;
  end_date_time?: string;
  application_start_date?: string;
  application_end_date?: string;
}

export interface MentorProgram {
  id: string;
  name: string;
  slug: string;
  description?: string;
  logo_url?: string;
  skills?: string[];
  terms?: MentorProgramTerm[];
  mentors?: MentorApiMentor[];
}

export interface MentorItem {
  user_id: string;
  name?: string;
  avatar_url?: string;
  introduction?: string;
  skills?: string[];
  joined_at: string;
}

export interface MentorApiMentee {
  user_id: string;
  name?: string;
  avatar_url?: string;
  introduction?: string;
  program_name: string;
  term_name: string;
  status: string;
}

export interface MentorApiStats {
  programs_mentoring: number;
  current_mentees: number;
  mentees_graduated: number;
}

export interface MentorApiDetail extends MentorItem {
  github_url?: string;
  linkedin_url?: string;
  programs?: MentorProgram[];
  current_mentees?: MentorApiMentee[];
  graduated_mentees?: MentorApiMentee[];
  stats?: MentorApiStats;
}

export interface MentorListResponse {
  data: MentorItem[];
  meta: { total: number; limit: number; offset: number };
}

export interface MentorSummary {
  mentor_count: number;
  program_count: number;
}

function fetchErrorStatus(error: unknown): number {
  if (typeof error === 'object' && error !== null && 'statusCode' in error) {
    const statusCode = Number((error as { statusCode?: number }).statusCode);
    if (Number.isFinite(statusCode) && statusCode > 0) return statusCode;
  }
  return 502;
}

function formatSinceLabel(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  const month = date.toLocaleString('en-US', { month: 'short' });
  return `Since ${month}. ${date.getFullYear()}`;
}

function formatDateRange(start?: string, end?: string): string {
  const startDate = start ? new Date(start) : undefined;
  const endDate = end ? new Date(end) : undefined;
  const format = (value: Date) => value.toLocaleString('en-US', { month: 'short', year: 'numeric' });
  if (
    startDate &&
    !Number.isNaN(startDate.getTime()) &&
    endDate &&
    !Number.isNaN(endDate.getTime())
  ) {
    return `${format(startDate)} - ${format(endDate)}`;
  }
  if (startDate && !Number.isNaN(startDate.getTime())) {
    return format(startDate);
  }
  return '';
}

function logoInitials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length >= 2) {
    return `${parts[0]![0] ?? ''}${parts[1]![0] ?? ''}`.toUpperCase();
  }
  return name.trim().slice(0, 2).toUpperCase();
}

function toDate(iso?: string): string {
  if (!iso) return '';
  return iso.slice(0, 10);
}

function mapCatalogTerm(term: MentorProgramTerm): ProgramTerm {
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

function mapProgramTerm(term: MentorProgramTerm): ProfileProgramTerm {
  return {
    id: term.id,
    label: term.name,
    dateRangeLabel: formatDateRange(term.start_date_time, term.end_date_time),
  };
}

function mapProgram(program: MentorProgram): ProfileProgram {
  const terms = program.terms ?? [];
  return {
    id: program.id,
    title: program.name,
    description: program.description ?? '',
    foundationLabel: '',
    status: toProgramCardStatus(terms.map(mapCatalogTerm)),
    skills: program.skills ?? [],
    terms: terms.map(mapProgramTerm),
    mentors: (program.mentors ?? []).map((mentor) => ({
      id: mentor.user_id,
      name: mentor.name?.trim() || 'Mentor',
      title: mentor.introduction?.trim() || undefined,
      avatarUrl: mentor.avatar_url,
    })),
    logoInitials: logoInitials(program.name),
    logoUrl: program.logo_url,
  };
}

function mapMentee(mentee: MentorApiMentee): MentorMenteeSummary {
  const programLabel = [mentee.program_name, mentee.term_name].filter(Boolean).join(' · ');
  return {
    id: mentee.user_id,
    name: mentee.name?.trim() || 'Mentee',
    bio: mentee.introduction?.trim() || '',
    programLabel,
    avatarUrl: mentee.avatar_url,
  };
}

function mapStats(stats?: MentorApiStats): MentorStats {
  return {
    programsMentoring: stats?.programs_mentoring ?? 0,
    currentMentees: stats?.current_mentees ?? 0,
    menteesGraduated: stats?.mentees_graduated ?? 0,
  };
}

export function mapMentorItem(item: MentorItem): Mentor {
  return {
    id: item.user_id,
    name: item.name?.trim() || 'Mentor',
    bio: item.introduction?.trim() || '',
    skills: item.skills ?? [],
    sinceLabel: formatSinceLabel(item.joined_at),
    joinedAt: item.joined_at,
    avatarUrl: item.avatar_url,
  };
}

export function mapMentorDetail(detail: MentorApiDetail): MentorDetail {
  const programs = (detail.programs ?? []).map(mapProgram);
  return {
    ...mapMentorItem(detail),
    affiliationsLabel: programs.map((program) => program.title).filter(Boolean).join(', '),
    githubUrl: detail.github_url,
    linkedinUrl: detail.linkedin_url,
    stats: mapStats(detail.stats),
    programs,
    currentMentees: (detail.current_mentees ?? []).map(mapMentee),
    graduatedMentees: (detail.graduated_mentees ?? []).map(mapMentee),
  };
}

export async function fetchMentors(query: {
  search?: string;
  skill?: string;
  limit?: number;
  offset?: number;
}): Promise<MentorListResponse> {
  const config = useRuntimeConfig();
  try {
    return await $fetch<MentorListResponse>(`${config.apiBaseUrl}/v1/mentors`, {
      query: {
        search: query.search || undefined,
        skill: query.skill && query.skill !== 'all' ? query.skill : undefined,
        limit: query.limit ?? 20,
        offset: query.offset ?? 0,
      },
    });
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: 'Failed to load mentors',
    });
  }
}

export async function fetchMentor(id: string): Promise<MentorApiDetail> {
  const config = useRuntimeConfig();
  try {
    return await $fetch<MentorApiDetail>(`${config.apiBaseUrl}/v1/mentors/${id}`);
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: fetchErrorStatus(error) === 404 ? 'Mentor not found' : 'Failed to load mentor',
    });
  }
}

export function toMentorsListResponse(page: MentorListResponse): MentorsListResponse {
  return {
    data: (page.data ?? []).map(mapMentorItem),
    total: page.meta.total,
  };
}

export async function fetchMentorSummary(): Promise<MentorsSummaryResponse> {
  const config = useRuntimeConfig();
  try {
    const summary = await $fetch<MentorSummary>(`${config.apiBaseUrl}/v1/mentors/summary`);
    return {
      mentorCount: summary.mentor_count,
      programCount: summary.program_count,
    };
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: 'Failed to load mentor summary',
    });
  }
}
