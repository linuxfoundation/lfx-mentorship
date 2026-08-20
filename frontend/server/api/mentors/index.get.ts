// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { SKILL_LIST } from '../../../app/config/skills';
import { MOCK_MENTORS } from '../../mock-data/directory';
import type { Mentor, MentorsListResponse } from '../../../app/types/mentor.types';

export default defineEventHandler((event): MentorsListResponse => {
  const query = getQuery(event);
  const search = String(query.search ?? '')
    .trim()
    .toLowerCase();
  const skill = String(query.skill ?? 'all');

  let data: Mentor[] = [...MOCK_MENTORS];

  if (skill && skill !== 'all') {
    data = data.filter((mentor) =>
      mentor.skills.some((item) => item.toLowerCase() === skill.toLowerCase()),
    );
  }

  if (search) {
    data = data.filter((mentor) => {
      const haystack = [mentor.name, mentor.bio, ...mentor.skills, ...mentor.projects]
        .join(' ')
        .toLowerCase();
      return haystack.includes(search);
    });
  }

  data = data.sort((a, b) => a.name.localeCompare(b.name));

  const projectCount = new Set(MOCK_MENTORS.flatMap((mentor) => mentor.projects)).size;

  return {
    data,
    total: data.length,
    skills: SKILL_LIST,
    mentorCount: MOCK_MENTORS.length,
    projectCount,
  };
});
