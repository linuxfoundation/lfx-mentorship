// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { SKILL_LIST } from '../../../app/config/skills';
import { MOCK_PROGRAMS } from '../../mock-data/programs';
import type {
  Program,
  ProgramSortBy,
  ProgramStatus,
  ProgramsListResponse,
} from '../../../app/types/program.types';
import { PROGRAM_STATUSES } from '../../../app/types/program.types';

function isProgramStatus(value: string): value is ProgramStatus {
  return (PROGRAM_STATUSES as readonly string[]).includes(value);
}


const STATUS_RANK_ACCEPTING_FIRST: Record<ProgramStatus, number> = {
  acceptance: 0,
  'in-progress': 1,
  completed: 2,
};

const STATUS_RANK_COMPLETED_FIRST: Record<ProgramStatus, number> = {
  completed: 0,
  'in-progress': 1,
  acceptance: 2,
};

function sortPrograms(data: Program[], sortBy: ProgramSortBy): Program[] {
  const sorted = [...data];

  switch (sortBy) {
    case 'accepting_first':
      return sorted.sort(
        (a, b) =>
          STATUS_RANK_ACCEPTING_FIRST[a.status] - STATUS_RANK_ACCEPTING_FIRST[b.status] ||
          a.name.localeCompare(b.name),
      );
    case 'completed_first':
      return sorted.sort(
        (a, b) =>
          STATUS_RANK_COMPLETED_FIRST[a.status] - STATUS_RANK_COMPLETED_FIRST[b.status] ||
          a.name.localeCompare(b.name),
      );
    case 'name_asc':
      return sorted.sort((a, b) => a.name.localeCompare(b.name));
    case 'name_desc':
      return sorted.sort((a, b) => b.name.localeCompare(a.name));
    case 'updated_oldest':
      return sorted.sort(
        (a, b) => new Date(a.updatedAt).getTime() - new Date(b.updatedAt).getTime(),
      );
    case 'updated_newest':
      return sorted.sort(
        (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
      );
    default:
      return sorted;
  }
}

export default defineEventHandler((event): ProgramsListResponse => {
  const query = getQuery(event);
  const search = String(query.search ?? '')
    .trim()
    .toLowerCase();
  const skill = String(query.skill ?? 'all');
  const status = String(query.status ?? 'all');
  const sortBy = String(query.sortBy ?? 'accepting_first') as ProgramSortBy;

  let data = [...MOCK_PROGRAMS];

  if (status !== 'all' && isProgramStatus(status)) {
    data = data.filter((program) => program.status === status);
  }

  if (skill && skill !== 'all') {
    data = data.filter((program) =>
      program.skills.some((item) => item.toLowerCase() === skill.toLowerCase()),
    );
  }

  if (search) {
    data = data.filter((program) => {
      const haystack = [
        program.name,
        program.description,
        program.foundation.name,
        program.activeTerm.name,
        ...program.skills,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(search);
    });
  }

  data = sortPrograms(data, sortBy);

  const foundationCount = new Set(MOCK_PROGRAMS.map((program) => program.foundation.id)).size;

  return {
    data,
    total: data.length,
    skills: SKILL_LIST,
    programCount: MOCK_PROGRAMS.length,
    foundationCount,
  };
});
