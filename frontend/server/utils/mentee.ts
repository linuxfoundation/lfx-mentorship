// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type {
  DirectoryMentorRef,
  Mentee,
  MenteeDetail,
  MenteeStatus,
  MenteesListResponse,
  MenteesSummaryResponse,
  ProfileProgram,
  ProfileProgramStatus,
  ProfileProgramTerm,
} from '../../app/types/mentee.types';

export interface MenteeMentor {
  id: string;
  user_id: string;
  name?: string;
  avatar_url?: string;
  introduction?: string;
}

export interface MenteeProject {
  id: string;
  name: string;
  slug: string;
  logo_url?: string;
}

export interface MenteeProgramTerm {
  id: string;
  name: string;
  start_date_time?: string;
  end_date_time?: string;
  application_status: string;
}

export interface MenteeProgram {
  id: string;
  name: string;
  slug: string;
  description?: string;
  logo_url?: string;
  status: string;
  skills?: string[];
  terms?: MenteeProgramTerm[];
  mentors?: MenteeMentor[];
}

export interface MenteeItem {
  user_id: string;
  name?: string;
  avatar_url?: string;
  introduction?: string;
  skills?: string[];
  status?: string;
  joined_at: string;
  program?: MenteeProject;
  mentors?: MenteeMentor[];
}

export interface MenteeApiDetail extends MenteeItem {
  github_url?: string;
  linkedin_url?: string;
  programs?: MenteeProgram[];
}

export interface MenteeListResponse {
  data: MenteeItem[];
  meta: { total: number; limit: number; offset: number };
}

export interface MenteeSummary {
  mentee_count: number;
  program_count: number;
}

function fetchErrorStatus(error: unknown): number {
  if (typeof error === 'object' && error !== null && 'statusCode' in error) {
    const statusCode = Number((error as { statusCode?: number }).statusCode);
    if (Number.isFinite(statusCode) && statusCode > 0) return statusCode;
  }
  return 502;
}

function toMenteeStatus(status?: string): MenteeStatus | undefined {
  if (!status) return undefined;
  switch (status) {
    case 'graduated':
      return 'graduated';
    case 'active':
    case 'accepted':
      return 'active';
    default:
      return undefined;
  }
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
  const format = (value: Date) =>
    value.toLocaleString('en-US', { month: 'short', year: 'numeric' });
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

function mapMentor(mentor: MenteeMentor): DirectoryMentorRef {
  return {
    id: mentor.user_id,
    name: mentor.name?.trim() || 'Mentor',
    title: mentor.introduction?.trim() || undefined,
    avatarUrl: mentor.avatar_url,
  };
}

function mapProgramStatus(status: string): ProfileProgramStatus {
  return status === 'graduated' ? 'graduated' : 'active';
}

function mapProgramTerm(term: MenteeProgramTerm): ProfileProgramTerm {
  return {
    id: term.id,
    label: term.name,
    dateRangeLabel: formatDateRange(term.start_date_time, term.end_date_time),
  };
}

function mapProgram(program: MenteeProgram): ProfileProgram {
  return {
    id: program.id,
    title: program.name,
    description: program.description ?? '',
    foundationLabel: '',
    status: mapProgramStatus(program.status),
    skills: program.skills ?? [],
    terms: (program.terms ?? []).map(mapProgramTerm),
    mentors: (program.mentors ?? []).map(mapMentor),
    logoInitials: logoInitials(program.name),
    logoUrl: program.logo_url,
  };
}

export function mapMenteeItem(item: MenteeItem): Mentee {
  return {
    id: item.user_id,
    name: item.name?.trim() || 'Mentee',
    introduction: item.introduction?.trim() || '',
    skills: item.skills ?? [],
    status: toMenteeStatus(item.status),
    sinceLabel: formatSinceLabel(item.joined_at),
    joinedAt: item.joined_at,
    program: item.program
      ? {
          id: item.program.id,
          name: item.program.name,
          foundationLabel: '',
        }
      : {
          id: '',
          name: '',
          foundationLabel: '',
        },
    mentors: (item.mentors ?? []).map(mapMentor),
    avatarUrl: item.avatar_url,
  };
}

export function mapMenteeDetail(detail: MenteeApiDetail): MenteeDetail {
  return {
    ...mapMenteeItem(detail),
    githubUrl: detail.github_url,
    linkedinUrl: detail.linkedin_url,
    programs: (detail.programs ?? []).map(mapProgram),
  };
}

export async function fetchMentees(query: {
  search?: string;
  skill?: string;
  status?: string;
  limit?: number;
  offset?: number;
}): Promise<MenteeListResponse> {
  const config = useRuntimeConfig();
  try {
    return await $fetch<MenteeListResponse>(`${config.apiBaseUrl}/v1/mentees`, {
      query: {
        search: query.search || undefined,
        skill: query.skill && query.skill !== 'all' ? query.skill : undefined,
        status: query.status && query.status !== 'all' ? query.status : undefined,
        limit: query.limit ?? 20,
        offset: query.offset ?? 0,
      },
    });
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: 'Failed to load mentees',
    });
  }
}

export async function fetchMentee(id: string): Promise<MenteeApiDetail> {
  const config = useRuntimeConfig();
  try {
    return await $fetch<MenteeApiDetail>(`${config.apiBaseUrl}/v1/mentees/${id}`);
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: fetchErrorStatus(error) === 404 ? 'Mentee not found' : 'Failed to load mentee',
    });
  }
}

export function toMenteesListResponse(page: MenteeListResponse): MenteesListResponse {
  return {
    data: (page.data ?? []).map(mapMenteeItem),
    total: page.meta.total,
  };
}

export async function fetchMenteeSummary(): Promise<MenteesSummaryResponse> {
  const config = useRuntimeConfig();
  try {
    const summary = await $fetch<MenteeSummary>(`${config.apiBaseUrl}/v1/mentees/summary`);
    return {
      menteeCount: summary.mentee_count,
      programCount: summary.program_count,
    };
  } catch (error) {
    throw createError({
      statusCode: fetchErrorStatus(error),
      message: 'Failed to load mentee summary',
    });
  }
}
