// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { SKILL_LIST } from '../../../app/config/skills';
import { MOCK_MENTEES } from '../../mock-data/directory';
import type { Mentee, MenteeStatus, MenteesListResponse } from '../../../app/types/mentee.types';
import { MENTEE_STATUSES } from '../../../app/types/mentee.types';

function isMenteeStatus(value: string): value is MenteeStatus {
  return (MENTEE_STATUSES as readonly string[]).includes(value);
}

export default defineEventHandler((event): MenteesListResponse => {
  const query = getQuery(event);
  const search = String(query.search ?? '')
    .trim()
    .toLowerCase();
  const skill = String(query.skill ?? 'all');
  const status = String(query.status ?? 'all');

  let data: Mentee[] = [...MOCK_MENTEES];

  if (status !== 'all' && isMenteeStatus(status)) {
    data = data.filter((mentee) => mentee.status === status);
  }

  if (skill && skill !== 'all') {
    data = data.filter((mentee) =>
      mentee.skills.some((item) => item.toLowerCase() === skill.toLowerCase()),
    );
  }

  if (search) {
    data = data.filter((mentee) => {
      const haystack = [
        mentee.name,
        mentee.bio,
        mentee.project.name,
        mentee.project.foundationLabel,
        ...mentee.skills,
        ...mentee.mentors.map((mentor) => mentor.name),
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(search);
    });
  }

  data = data.sort((a, b) => a.name.localeCompare(b.name));

  const projectCount = new Set(MOCK_MENTEES.map((mentee) => mentee.project.id)).size;

  return {
    data,
    total: data.length,
    skills: SKILL_LIST,
    menteeCount: MOCK_MENTEES.length,
    projectCount,
  };
});
