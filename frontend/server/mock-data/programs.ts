// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type {
  Foundation,
  Program,
  ProgramMentee,
  ProgramTerm,
  TermStatus,
} from '../../app/types/program.types';
import type { MenteeStatus } from '../../app/types/mentee.types';
import { withActiveTerms } from '../../app/utils/program-terms';

export const MOCK_FOUNDATIONS: Foundation[] = [
  { id: 'f-cncf', name: 'CNCF', slug: 'cncf' },
  { id: 'f-lf', name: 'Linux Foundation', slug: 'linux-foundation' },
  { id: 'f-lfai', name: 'LF AI & Data', slug: 'lf-ai-data' },
  {
    id: 'f-long',
    name: 'Cloud Native Computing Foundation Special Interest Group for API Artifact Tooling and Developer Experience',
    slug: 'cncf-api-artifact-tooling-dx',
  },
];

const cncf = MOCK_FOUNDATIONS[0]!;
const lf = MOCK_FOUNDATIONS[1]!;
const lfai = MOCK_FOUNDATIONS[2]!;
const longFoundation = MOCK_FOUNDATIONS[3]!;

const TERM_DATES_BY_LABEL: Record<string, { startsAt: string; endsAt: string }> = {
  'Jan-Mar 2026': { startsAt: '2026-01-01', endsAt: '2026-03-31' },
  'Apr-Jun 2026': { startsAt: '2026-04-01', endsAt: '2026-06-30' },
  'Sep-Nov 2026': { startsAt: '2026-09-01', endsAt: '2026-11-30' },
  'Jan-Mar 2027': { startsAt: '2027-01-01', endsAt: '2027-03-31' },
  'Jan-Mar 2025': { startsAt: '2025-01-01', endsAt: '2025-03-31' },
  'Apr-Jun 2025': { startsAt: '2025-04-01', endsAt: '2025-06-30' },
  'Sep-Nov 2025': { startsAt: '2025-09-01', endsAt: '2025-11-30' },
  'Mar 2 – May 25, 2026': { startsAt: '2026-03-02', endsAt: '2026-05-25' },
  'Jun 1 – Aug 24, 2026': { startsAt: '2026-06-01', endsAt: '2026-08-24' },
  'Sep 1 – Nov 23, 2026': { startsAt: '2026-09-01', endsAt: '2026-11-23' },
  'Dec 1, 2026 – Feb 22, 2027': { startsAt: '2026-12-01', endsAt: '2027-02-22' },
  'September through November 2026 (extended mentorship window)': {
    startsAt: '2026-09-01',
    endsAt: '2026-11-30',
  },
};

function term(
  id: string,
  name: string,
  dateRangeLabel: string,
  applicationsCloseAt: string,
  status: TermStatus = 'open',
  applicationsStartAt?: string,
): ProgramTerm {
  const dates = TERM_DATES_BY_LABEL[dateRangeLabel] ?? {
    startsAt: '2026-01-01',
    endsAt: '2026-12-31',
  };
  const closeMs = new Date(applicationsCloseAt).getTime();
  const start = applicationsStartAt ?? new Date(closeMs - 60 * 24 * 60 * 60 * 1000).toISOString();

  return {
    id,
    name,
    status,
    dateRangeLabel,
    applicationsStartAt: start,
    applicationsCloseAt,
    ...dates,
  };
}

function emailFromName(name: string): string {
  const ascii = name.normalize('NFD').replace(/[\u0300-\u036f]/g, '');
  const parts = ascii.split(/[\s-]+/).filter(Boolean);
  const first = parts[0]?.charAt(0) ?? '';
  const last = parts[parts.length - 1] ?? 'mentee';
  return `${first}${last}@example.org`.toLowerCase();
}

function mentee(
  id: string,
  name: string,
  status: MenteeStatus,
  termLabel: string,
  intro: string,
  email?: string,
): ProgramMentee {
  return {
    id,
    name,
    status,
    termLabel,
    intro,
    email: email ?? emailFromName(name),
  };
}

const PROGRAM_SEEDS: Omit<Program, 'activeTerms'>[] = [
  {
    id: '1',
    slug: 'kubernetes-contributors',
    name: 'Kubernetes Contributors',
    description:
      'A mentorship track helping new contributors learn Kubernetes contribution workflows, SIGs, and community practices.',
    logoUrl: 'https://cdn.platform.linuxfoundation.org/assets/lf-favicon.png',
    skills: ['GO', 'Kubernetes', 'Git', 'Continuous integration', 'Docker'],
    status: 'acceptance',
    foundation: cncf,
    terms: [
      term(
        't1-1',
        'Spring 2026',
        'Mar 2 – May 25, 2026',
        '2026-01-15T00:00:00.000Z',
        'closed',
        '2025-11-03T00:00:00.000Z',
      ),
      term(
        't1-2',
        'Summer 2026',
        'Jun 1 – Aug 24, 2026',
        '2026-04-15T00:00:00.000Z',
        'closed',
        '2026-02-02T00:00:00.000Z',
      ),
      term(
        't1-3',
        'Fall 2026',
        'Sep 1 – Nov 23, 2026',
        '2026-09-15T00:00:00.000Z',
        'open',
        '2026-05-01T00:00:00.000Z',
      ),
      term(
        't1-4',
        'Winter 2026',
        'Dec 1, 2026 – Feb 22, 2027',
        '2026-10-15T00:00:00.000Z',
        'open',
        '2026-09-01T00:00:00.000Z',
      ),
    ],
    updatedAt: '2026-08-10T12:00:00.000Z',
    repositoryUrl: 'https://github.com/kubernetes/community',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-1',
        'Hana Suzuki',
        'active',
        'Fall 2026',
        'New Kubernetes contributor learning SIG workflows, issue triage, and how to land a first patch with mentor support.',
        'hsuzuki@example.org',
      ),
      mentee(
        'me-2',
        'Mateo Rossi',
        'active',
        'Fall 2026',
        'Exploring Kubernetes contribution paths and helping improve SIG docs so newcomers can find the right working group.',
        'mrossi@example.org',
      ),
      mentee(
        'me-3',
        'Ifeoma Adeyemi',
        'graduated',
        'Spring 2026',
        'Practiced Git-based review and community norms while contributing small fixes across Kubernetes SIGs.',
        'iadeyemi@example.org',
      ),
      mentee(
        'me-4',
        'Luis Fernández',
        'graduated',
        'Spring 2026',
        'Shipped contributor-docs improvements and supported SIG onboarding for first-time Kubernetes mentees.',
        'lfernandez@example.org',
      ),
      mentee(
        'me-5',
        'Grace Wanjiru',
        'graduated',
        'Summer 2025',
        'Helped with issue triage and first-patch reviews during an earlier Kubernetes mentorship term.',
        'gwanjiru@example.org',
      ),
      mentee(
        'me-6',
        'Omar Haddad',
        'graduated',
        'Spring 2025',
        'Contributed small SIG fixes and learned Kubernetes community process with mentor support.',
        'ohaddad@example.org',
      ),
    ],
    mentors: [
      {
        id: 't1',
        name: 'Priya Shah',
        intro:
          'Kubernetes SIG contributor who mentors newcomers on community process, issue triage, and first pull requests.',
      },
      {
        id: 't2',
        name: 'Chris Okonkwo',
        intro:
          'Kubernetes maintainer helping mentees with code review, SIG participation, and upstream collaboration.',
      },
    ],
    sponsors: [
      { id: 's1', name: 'Equinix', amountCents: 2400000 },
      { id: 's2', name: 'Red Hat', amountCents: 1800000 },
      { id: 's2b', name: 'Google', amountCents: 1200000 },
      { id: 's2c', name: 'Individual donors', amountCents: 415000 },
    ],
  },
  {
    id: '2',
    slug: 'open-source-security',
    name: 'Open Source Security',
    description:
      'Pair with security mentors to harden projects, triage vulnerabilities, and build secure-by-default habits.',
    logoUrl: 'https://cdn.platform.linuxfoundation.org/assets/lf-favicon.png',
    skills: ['Security', 'Rust', 'Code quality'],
    status: 'in-progress',
    foundation: cncf,
    terms: [
      term('t2-1', 'Term 1', 'Jan-Mar 2026', '2025-11-20T00:00:00.000Z', 'closed'),
      term('t2-2', 'Term 2', 'Apr-Jun 2026', '2026-02-28T00:00:00.000Z'),
    ],
    updatedAt: '2026-08-01T09:30:00.000Z',
    repositoryUrl: 'https://github.com/ossf',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-4',
        'Taylor Brooks',
        'active',
        'Summer 2026',
        'Learning vulnerability triage and secure-by-default habits while pairing with security mentors on real project issues.',
      ),
      mentee(
        'me-5',
        'Morgan Diaz',
        'graduated',
        'Spring 2026',
        'Hardened CI pipelines and studied OpenSSF practices to help projects ship with stronger supply-chain checks.',
      ),
    ],
    mentors: [
      {
        id: 't3',
        name: 'Nina Patel',
        intro:
          'Security engineer mentoring mentees on vulnerability triage, secure defaults, and OpenSSF tooling.',
      },
      {
        id: 't4',
        name: 'Ethan Cole',
        intro:
          'OpenSSF mentor helping contributors build supply-chain security skills and review hardening work.',
      },
      {
        id: 't5',
        name: 'Riley Quinn',
        intro:
          'Project maintainer guiding mentees through security issue handling and upstream patch review.',
      },
    ],
    sponsors: [{ id: 's3', name: 'OpenSSF', amountCents: 1500000 }],
  },
  {
    id: '3',
    slug: 'docs-and-developer-experience',
    name: 'Docs & Developer Experience',
    description:
      'Improve documentation, onboarding guides, and DX tooling with experienced open source maintainers.',
    skills: ['Documentation', 'Markdown', 'Vue.js', 'Front end'],
    status: 'completed',
    foundation: lf,
    terms: [term('t3-1', 'Term 1', 'Jan-Mar 2026', '2025-12-10T00:00:00.000Z', 'closed')],
    updatedAt: '2026-06-15T16:00:00.000Z',
    repositoryUrl: 'https://github.com/linuxfoundation',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-6',
        'Casey Ng',
        'graduated',
        'Spring 2026',
        'Improved documentation and onboarding guides so new contributors could find examples and land a first PR faster.',
      ),
    ],
    mentors: [
      {
        id: 't6',
        name: 'Avery Kim',
        intro:
          'Docs lead mentoring mentees on information architecture, contributor guides, and developer experience.',
      },
    ],
    sponsors: [{ id: 's4', name: 'Linux Foundation', amountCents: 2000000 }],
  },
  {
    id: '4',
    slug: 'apicurio-registry-overflow-fixture',
    name: 'Apicurio Registry: Prompt Template Playground for Cloud-Native API Artifact Management and Developer Experience ApicurioRegistryPromptTemplatePlaygroundForCloudNativeAPIArtifactManagement',
    description:
      'Build and improve a prompt-template playground around Apicurio Registry for cloud-native developers, including schema discovery, template authoring, validation workflows, and contributor onboarding. This copy is intentionally long so list cards can verify two-line clamping, wrapping, and overflow. Extra context: teams use the playground to experiment with AsyncAPI, OpenAPI, and Protobuf artifacts before publishing them to a shared registry used across multiple services and organizations.',
    logoUrl: 'https://cdn.platform.linuxfoundation.org/assets/lf-favicon.png',
    skills: [
      'GO',
      'API',
      'React',
      'GraphQL',
      'JSON',
      'REST API',
      'Documentation',
      'Markdown',
      'Vue.js',
      'TypeScript',
      'JavaScript',
      'Docker',
      'Kubernetes',
      'XML',
    ],
    status: 'acceptance',
    foundation: longFoundation,
    terms: [
      term('t4-1', 'Term 1', 'Jan-Mar 2026', '2025-12-01T00:00:00.000Z', 'closed'),
      term('t4-2', 'Term 2', 'Apr-Jun 2026', '2026-03-01T00:00:00.000Z', 'closed'),
      term(
        't4-3',
        'Term 3 — Cloud Native API Artifact Playground and Contributor Onboarding Cycle',
        'September through November 2026 (extended mentorship window)',
        '2026-09-15T00:00:00.000Z',
        'open',
        '2026-07-01T00:00:00.000Z',
      ),
    ],
    updatedAt: '2026-08-12T08:00:00.000Z',
    repositoryUrl: 'https://github.com/Apicurio/apicurio-registry',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-7',
        'Jamie Soto-Fernandez de la Vega y Contreras',
        'active',
        'Fall 2026',
        'Building a prompt-template playground around Apicurio Registry, including schema discovery, template authoring, and validation workflows for cloud-native API artifacts.',
      ),
      mentee(
        'me-8',
        'Robin Hale',
        'active',
        'Fall 2026',
        'Working on template authoring and validation flows so developers can experiment with AsyncAPI, OpenAPI, and Protobuf artifacts before publishing.',
      ),
      mentee(
        'me-9',
        'DrewParksApicurioRegistryContributorWithoutSpaces',
        'graduated',
        'Summer 2026',
        'Contributed registry UX and contributor onboarding so teams can find, validate, and share API artifacts across services.',
      ),
      mentee(
        'me-10',
        'Skyler Dunn',
        'graduated',
        'Spring 2026',
        'Helped with contributor onboarding and playground docs for Apicurio Registry’s cloud-native artifact workflows.',
      ),
    ],
    mentors: [
      {
        id: 't7',
        name: 'Cameron West-Nakamura',
        intro:
          'Apicurio Registry maintainer mentoring mentees on cloud-native API artifact tooling, schema discovery, and contributor experience across the prompt-template playground.',
      },
      {
        id: 't7b',
        name: 'Lee Nakamura',
        intro:
          'CNCF mentor supporting mentees as they learn registry workflows, template validation, and upstream contribution on Apicurio.',
      },
    ],
    sponsors: [
      {
        id: 's5',
        name: 'Equinix Metal Cloud Native Infrastructure and Developer Experience Foundation',
        amountCents: 2400000,
      },
      { id: 's5b', name: 'Red Hat', amountCents: 1800000 },
      { id: 's6', name: 'Google', amountCents: 1200000 },
      { id: 's6b', name: 'IndividualDonorsWithoutSpacesForOverflowChecks', amountCents: 415000 },
    ],
  },
  {
    id: '5',
    slug: 'linux-kernel-newcomers',
    name: 'Linux Kernel Newcomers',
    description:
      'Guided introduction to kernel development, mailing lists, and first-patch contribution paths.',
    skills: ['C', 'Linux', 'Git'],
    status: 'in-progress',
    foundation: lf,
    terms: [
      term('t5-1', 'Term 1', 'Jan-Mar 2026', '2025-11-15T00:00:00.000Z', 'closed'),
      term('t5-2', 'Term 2', 'Apr-Jun 2026', '2026-02-15T00:00:00.000Z'),
    ],
    updatedAt: '2026-07-20T11:00:00.000Z',
    repositoryUrl: 'https://git.kernel.org',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-11',
        'Parker James',
        'active',
        'Summer 2026',
        'Learning kernel development, mailing-list etiquette, and how to send a first patch with mentor review.',
      ),
      mentee(
        'me-12',
        'Reese Ortiz',
        'graduated',
        'Spring 2026',
        'Practiced C and Git workflows for kernel contribution and followed subsystem conventions with mentors.',
      ),
    ],
    mentors: [
      {
        id: 't8',
        name: 'Harper Stone',
        intro:
          'Kernel mentor helping newcomers through mailing-list etiquette, patch format, and first-patch reviews.',
      },
      {
        id: 't9',
        name: 'Quinn Adler',
        intro:
          'Kernel maintainer mentoring contributors on subsystem conventions, patch series, and upstream review.',
      },
    ],
    sponsors: [{ id: 's7', name: 'Linux Foundation', amountCents: 1200000 }],
  },
  {
    id: '6',
    slug: 'community-leadership',
    name: 'Community Leadership',
    description:
      'Develop facilitation, governance, and inclusive community leadership skills with LF mentors.',
    logoUrl: 'https://cdn.platform.linuxfoundation.org/assets/lf-favicon.png',
    skills: ['Project management', 'Support', 'Documentation'],
    status: 'completed',
    foundation: lfai,
    terms: [
      term('t6-1', 'Term 1', 'Jan-Mar 2025', '2024-12-01T00:00:00.000Z', 'closed'),
      term('t6-2', 'Term 2', 'Apr-Jun 2025', '2025-03-01T00:00:00.000Z', 'closed'),
      term('t6-3', 'Term 3', 'Sep-Nov 2025', '2025-07-15T00:00:00.000Z', 'closed'),
    ],
    updatedAt: '2026-05-01T10:00:00.000Z',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-13',
        'Blake Foster',
        'graduated',
        'Fall 2025',
        'Developed facilitation and inclusive community practices through hands-on work with LF mentors.',
      ),
      mentee(
        'me-14',
        'Cameron Ellis',
        'graduated',
        'Summer 2025',
        'Learned open source governance and community leadership by supporting contributor programs.',
      ),
      mentee(
        'me-15',
        'Devon Price',
        'graduated',
        'Spring 2025',
        'Built skills in community operations, facilitation, and helping new contributors feel welcome.',
      ),
    ],
    mentors: [
      {
        id: 't10',
        name: 'Sage Monroe',
        intro:
          'Community lead mentoring mentees on facilitation, governance, and inclusive leadership in open source.',
      },
      {
        id: 't11',
        name: 'Finley Hart',
        intro:
          'Program mentor supporting mentees as they practice community leadership and contributor engagement.',
      },
    ],
    sponsors: [{ id: 's8', name: 'LF AI & Data', amountCents: 900000 }],
  },
  {
    id: '7',
    slug: 'prometheus-observability',
    name: 'Prometheus Observability',
    description:
      'Extend Prometheus exporters and Grafana dashboards with mentors from the observability ecosystem.',
    logoUrl: 'https://cdn.platform.linuxfoundation.org/assets/lf-favicon.png',
    skills: ['GO', 'Monitoring', 'Kubernetes', 'Python'],
    status: 'acceptance',
    foundation: cncf,
    terms: [
      term(
        't7-1',
        'Term 3',
        'Sep-Nov 2026',
        '2026-09-30T00:00:00.000Z',
        'open',
        '2026-07-01T00:00:00.000Z',
      ),
    ],
    updatedAt: '2026-08-12T10:00:00.000Z',
    repositoryUrl: 'https://github.com/prometheus',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-16',
        'Amir Haddad',
        'active',
        'Fall 2026',
        'Extending Prometheus exporters and Grafana dashboards while learning observability patterns with mentors.',
      ),
    ],
    mentors: [
      {
        id: 't12',
        name: 'Dana Okafor',
        intro:
          'Staff engineer mentoring mentees on Prometheus exporters, Grafana dashboards, and production observability.',
      },
    ],
    sponsors: [{ id: 's9', name: 'CNCF', amountCents: 1100000 }],
  },
  {
    id: '8',
    slug: 'rust-for-systems',
    name: 'Rust for Systems',
    description:
      'Learn safe systems programming by contributing to Rust crates used across LF projects.',
    skills: ['Rust', 'C', 'Git', 'Testing'],
    status: 'in-progress',
    foundation: lf,
    terms: [
      term('t8-1', 'Term 1', 'Jan-Mar 2026', '2025-12-01T00:00:00.000Z', 'closed'),
      term('t8-2', 'Term 2', 'Apr-Jun 2026', '2026-03-01T00:00:00.000Z'),
    ],
    updatedAt: '2026-07-28T14:00:00.000Z',
    repositoryUrl: 'https://github.com/rust-lang',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      mentee(
        'me-17',
        'Lucia Romano',
        'active',
        'Summer 2026',
        'Learning safe systems programming by contributing to Rust crates used across Linux Foundation projects.',
      ),
      mentee(
        'me-18',
        'Ben Walsh',
        'graduated',
        'Spring 2026',
        'Practiced Rust testing and contribution workflows with mentors focused on systems programming.',
      ),
    ],
    mentors: [
      {
        id: 't13',
        name: 'Nina Patel',
        intro:
          'Security-minded systems engineer mentoring mentees on Rust, testing, and safe contribution habits.',
      },
    ],
    sponsors: [{ id: 's10', name: 'Linux Foundation', amountCents: 1000000 }],
  },
];

export const MOCK_PROGRAMS: Program[] = PROGRAM_SEEDS.map(withActiveTerms);
