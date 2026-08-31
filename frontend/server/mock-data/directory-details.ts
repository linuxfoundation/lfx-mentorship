// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { MenteeDetail, ProfileProgram } from '../../app/types/mentee.types';
import type { MentorDetail, MentorMenteeSummary } from '../../app/types/mentor.types';
import { MOCK_MENTEES, MOCK_MENTORS } from './directory';

const THANOS_PROGRAM: ProfileProgram = {
  id: 'pp-thanos',
  title: 'Thanos: Implement fan-out query observability',
  description:
    'Improve query-path tracing and dashboards so operators can diagnose fan-out latency across Thanos components.',
  foundationLabel: 'CNCF',
  status: 'graduated',
  skills: ['GO', 'React', 'TypeScript', 'Git'],
  terms: [
    {
      id: 'term-thanos-3',
      label: 'Term 3: Sep-Nov',
      dateRangeLabel: 'Sep 2023 - Nov 2023',
    },
  ],
  mentors: [
    { id: 'mo-1', name: 'Dana Okafor', title: 'Staff Engineer, Equinix' },
    { id: 'mo-9', name: 'Ravi Menon', title: 'Maintainer, Thanos' },
  ],
  logoInitials: 'TH',
};

const GRIDFLOW_PROGRAM: ProfileProgram = {
  id: 'pp-gridflow',
  title: 'GridFlow: Scheduling simulator for grid operators',
  description:
    'Build a scheduling simulator that helps grid operators evaluate demand-response scenarios safely.',
  foundationLabel: 'LF Energy',
  status: 'accepting',
  skills: ['Python', 'Rust', 'Git'],
  terms: [
    {
      id: 'term-grid-3',
      label: 'Term 3: Sep–Nov 2026',
      dateRangeLabel: 'Sep 2026 - Nov 2026',
    },
  ],
  logoInitials: 'GR',
};

const THANOS_MENTOR_PROGRAM: ProfileProgram = {
  ...THANOS_PROGRAM,
  status: 'closed',
  description:
    'Guide mentees shipping observability improvements across the Thanos query path and related docs.',
  mentors: undefined,
};

function menteeSummary(
  id: string,
  name: string,
  bio: string,
  programLabel: string,
): MentorMenteeSummary {
  return { id, name, bio, programLabel };
}

/** Rich profile payloads keyed by id. List cards use MOCK_MENTEES; detail uses this map. */
export const MOCK_MENTEE_DETAILS: Record<string, MenteeDetail> = {
  'me-1': {
    ...MOCK_MENTEES[0]!,
    introduction:
      'Final-year computer science student focused on distributed systems. I contribute to observability tooling and want to keep working on cloud native projects after graduation.',
    skills: ['GO', 'React', 'Monitoring', 'Kubernetes', 'TypeScript'],
    githubUrl: 'https://github.com/',
    linkedinUrl: 'https://www.linkedin.com/',
    programs: [THANOS_PROGRAM],
    mentors: [
      { id: 'mo-1', name: 'Dana Okafor', title: 'Staff Engineer, Equinix' },
      { id: 'mo-9', name: 'Ravi Menon', title: 'Maintainer, Thanos' },
    ],
  },
};

/** Rich mentor profile payloads keyed by id. */
export const MOCK_MENTOR_DETAILS: Record<string, MentorDetail> = {
  'mo-1': {
    ...MOCK_MENTORS[0]!,
    bio: 'Staff engineer mentoring on Golang services and observability. I focus on production debugging habits, clear RFCs, and helping mentees land lasting upstream contributions.',
    skills: ['GO', 'Kubernetes', 'Documentation', 'Python'],
    affiliationsLabel: 'LF Energy · GridFlow, CNCF · Thanos',
    projects: ['GridFlow', 'Thanos'],
    githubUrl: 'https://github.com/',
    linkedinUrl: 'https://www.linkedin.com/',
    stats: { programsMentoring: 2, currentMentees: 2, menteesGraduated: 4 },
    programs: [GRIDFLOW_PROGRAM, THANOS_MENTOR_PROGRAM],
    currentMentees: [
      menteeSummary(
        'me-extra-1',
        'Ifeoma Adeyemi',
        'Working on GridFlow scheduling scenarios and operator-facing docs for demand-response pilots.',
        'GridFlow · Term 3',
      ),
      menteeSummary(
        'me-2',
        'Diego Souza',
        'Improving CloudEvents tooling while pairing with Dana on API ergonomics and review habits.',
        'CloudEvents · Term 2',
      ),
    ],
    graduatedMentees: [
      menteeSummary(
        'me-1',
        'Hana Suzuki',
        'Shipped Thanos query-path observability improvements and contributor-facing dashboards…',
        'Thanos · Term 3',
      ),
      menteeSummary(
        'me-extra-2',
        'Grace Wanjiru',
        'Hardened GridFlow CI and wrote runbooks for first-time energy-systems contributors…',
        'GridFlow · Term 1',
      ),
      menteeSummary(
        'me-extra-3',
        'Ines Duarte',
        'Documented Thanos fan-out debugging paths and improved onboarding for new query-path…',
        'Thanos · Term 2',
      ),
      menteeSummary(
        'me-extra-4',
        'Ravi Menon',
        'Contributed store gateway metrics and mentored peers on production tracing patterns…',
        'Thanos · Term 1',
      ),
    ],
  },
};

function fallbackMenteeDetail(id: string): MenteeDetail | undefined {
  const mentee = MOCK_MENTEES.find((item) => item.id === id);
  if (!mentee) return undefined;

  const initials = mentee.program.name.slice(0, 2).toUpperCase();
  return {
    ...mentee,
    githubUrl: 'https://github.com/',
    linkedinUrl: 'https://www.linkedin.com/',
    programs: [
      {
        id: `pp-${mentee.program.id}`,
        title: `${mentee.program.name}: Mentorship program`,
        description: mentee.introduction,
        foundationLabel: mentee.program.foundationLabel,
        status: mentee.status === 'graduated' ? 'graduated' : 'active',
        skills: mentee.skills.slice(0, 4),
        terms: [
          {
            id: `term-${mentee.id}`,
            label: 'Term 1',
            dateRangeLabel: mentee.sinceLabel.replace(/^Since\s+/i, ''),
          },
        ],
        mentors: mentee.mentors,
        logoInitials: initials,
      },
    ],
  };
}

function fallbackMentorDetail(id: string): MentorDetail | undefined {
  const mentor = MOCK_MENTORS.find((item) => item.id === id);
  if (!mentor) return undefined;

  const affiliationsLabel = mentor.projects.join(', ');
  const relatedMentees = MOCK_MENTEES.filter((mentee) =>
    mentee.mentors.some((item) => item.id === mentor.id),
  );

  return {
    ...mentor,
    affiliationsLabel,
    githubUrl: 'https://github.com/',
    linkedinUrl: 'https://www.linkedin.com/',
    stats: {
      programsMentoring: mentor.projects.length,
      currentMentees: relatedMentees.filter((m) => m.status === 'active').length,
      menteesGraduated: relatedMentees.filter((m) => m.status === 'graduated').length,
    },
    programs: mentor.projects.map((projectName, index) => ({
      id: `pp-${mentor.id}-${index}`,
      title: `${projectName}: Mentorship program`,
      description: mentor.bio,
      foundationLabel: 'LF',
      status: index === 0 ? 'accepting' : 'closed',
      skills: mentor.skills.slice(0, 3),
      terms: [
        {
          id: `term-${mentor.id}-${index}`,
          label: 'Term 3: Sep–Nov',
          dateRangeLabel: 'Sep 2026 - Nov 2026',
        },
      ],
      logoInitials: projectName.slice(0, 2).toUpperCase(),
    })),
    currentMentees: relatedMentees
      .filter((m) => m.status === 'active')
      .map((m) =>
        menteeSummary(m.id, m.name, m.introduction, `${m.program.name} · Term 3: Sep–Nov`),
      ),
    graduatedMentees: relatedMentees
      .filter((m) => m.status === 'graduated')
      .map((m) =>
        menteeSummary(m.id, m.name, m.introduction, `${m.program.name} · Term 3: Sep–Nov`),
      ),
  };
}

export function getMenteeDetail(id: string): MenteeDetail | undefined {
  return MOCK_MENTEE_DETAILS[id] ?? fallbackMenteeDetail(id);
}

export function getMentorDetail(id: string): MentorDetail | undefined {
  return MOCK_MENTOR_DETAILS[id] ?? fallbackMentorDetail(id);
}
