// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Foundation, Program, ProgramTerm } from '../../app/types/program.types';

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
  'Jan-Mar 2025': { startsAt: '2025-01-01', endsAt: '2025-03-31' },
  'Apr-Jun 2025': { startsAt: '2025-04-01', endsAt: '2025-06-30' },
  'Sep-Nov 2025': { startsAt: '2025-09-01', endsAt: '2025-11-30' },
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
): ProgramTerm {
  const dates = TERM_DATES_BY_LABEL[dateRangeLabel] ?? {
    startsAt: '2026-01-01',
    endsAt: '2026-12-31',
  };
  return { id, name, dateRangeLabel, applicationsCloseAt, ...dates };
}

export const MOCK_PROGRAMS: Program[] = [
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
      term('t1-1', 'Term 1', 'Jan-Mar 2026', '2025-12-01T00:00:00.000Z'),
      term('t1-2', 'Term 2', 'Apr-Jun 2026', '2026-03-01T00:00:00.000Z'),
      term('t1-3', 'Term 3', 'Sep-Nov 2026', '2026-07-15T00:00:00.000Z'),
    ],
    activeTerm: term('t1-3', 'Term 3', 'Sep-Nov 2026', '2026-07-15T00:00:00.000Z'),
    updatedAt: '2026-08-10T12:00:00.000Z',
    repositoryUrl: 'https://github.com/kubernetes/community',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm1',
        name: 'Alex Rivera',
        status: 'active',
        intro:
          'New Kubernetes contributor learning SIG workflows, issue triage, and how to land a first patch with mentor support.',
      },
      {
        id: 'm2',
        name: 'Sam Chen',
        status: 'active',
        intro:
          'Exploring Kubernetes contribution paths and helping improve SIG docs so newcomers can find the right working group.',
      },
      {
        id: 'm3',
        name: 'Jordan Lee',
        status: 'graduated',
        intro:
          'Practiced Git-based review and community norms while contributing small fixes across Kubernetes SIGs.',
      },
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
      term('t2-1', 'Term 1', 'Jan-Mar 2026', '2025-11-20T00:00:00.000Z'),
      term('t2-2', 'Term 2', 'Apr-Jun 2026', '2026-02-28T00:00:00.000Z'),
    ],
    activeTerm: term('t2-2', 'Term 2', 'Apr-Jun 2026', '2026-02-28T00:00:00.000Z'),
    updatedAt: '2026-08-01T09:30:00.000Z',
    repositoryUrl: 'https://github.com/ossf',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm4',
        name: 'Taylor Brooks',
        status: 'active',
        intro:
          'Learning vulnerability triage and secure-by-default habits while pairing with security mentors on real project issues.',
      },
      {
        id: 'm5',
        name: 'Morgan Diaz',
        status: 'graduated',
        intro:
          'Hardened CI pipelines and studied OpenSSF practices to help projects ship with stronger supply-chain checks.',
      },
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
    terms: [term('t3-1', 'Term 1', 'Jan-Mar 2026', '2025-12-10T00:00:00.000Z')],
    activeTerm: term('t3-1', 'Term 1', 'Jan-Mar 2026', '2025-12-10T00:00:00.000Z'),
    updatedAt: '2026-06-15T16:00:00.000Z',
    repositoryUrl: 'https://github.com/linuxfoundation',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm6',
        name: 'Casey Ng',
        status: 'graduated',
        intro:
          'Improved documentation and onboarding guides so new contributors could find examples and land a first PR faster.',
      },
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
      term('t4-1', 'Term 1', 'Jan-Mar 2026', '2025-12-01T00:00:00.000Z'),
      term('t4-2', 'Term 2', 'Apr-Jun 2026', '2026-03-01T00:00:00.000Z'),
      term(
        't4-3',
        'Term 3 — Cloud Native API Artifact Playground and Contributor Onboarding Cycle',
        'September through November 2026 (extended mentorship window)',
        '2026-07-15T00:00:00.000Z',
      ),
    ],
    activeTerm: term(
      't4-3',
      'Term 3 — Cloud Native API Artifact Playground and Contributor Onboarding Cycle',
      'September through November 2026 (extended mentorship window)',
      '2026-07-15T00:00:00.000Z',
    ),
    updatedAt: '2026-08-12T08:00:00.000Z',
    repositoryUrl: 'https://github.com/Apicurio/apicurio-registry',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm7',
        name: 'Jamie Soto-Fernandez de la Vega y Contreras',
        status: 'active',
        intro:
          'Building a prompt-template playground around Apicurio Registry, including schema discovery, template authoring, and validation workflows for cloud-native API artifacts.',
      },
      {
        id: 'm8',
        name: 'Robin Hale',
        status: 'active',
        intro:
          'Working on template authoring and validation flows so developers can experiment with AsyncAPI, OpenAPI, and Protobuf artifacts before publishing.',
      },
      {
        id: 'm9',
        name: 'DrewParksApicurioRegistryContributorWithoutSpaces',
        status: 'graduated',
        intro:
          'Contributed registry UX and contributor onboarding so teams can find, validate, and share API artifacts across services.',
      },
      {
        id: 'm10',
        name: 'Skyler Dunn',
        status: 'graduated',
        intro:
          'Helped with contributor onboarding and playground docs for Apicurio Registry’s cloud-native artifact workflows.',
      },
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
      term('t5-1', 'Term 1', 'Jan-Mar 2026', '2025-11-15T00:00:00.000Z'),
      term('t5-2', 'Term 2', 'Apr-Jun 2026', '2026-02-15T00:00:00.000Z'),
    ],
    activeTerm: term('t5-2', 'Term 2', 'Apr-Jun 2026', '2026-02-15T00:00:00.000Z'),
    updatedAt: '2026-07-20T11:00:00.000Z',
    repositoryUrl: 'https://git.kernel.org',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm11',
        name: 'Parker James',
        status: 'active',
        intro:
          'Learning kernel development, mailing-list etiquette, and how to send a first patch with mentor review.',
      },
      {
        id: 'm12',
        name: 'Reese Ortiz',
        status: 'graduated',
        intro:
          'Practiced C and Git workflows for kernel contribution and followed subsystem conventions with mentors.',
      },
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
      term('t6-1', 'Term 1', 'Jan-Mar 2025', '2024-12-01T00:00:00.000Z'),
      term('t6-2', 'Term 2', 'Apr-Jun 2025', '2025-03-01T00:00:00.000Z'),
      term('t6-3', 'Term 3', 'Sep-Nov 2025', '2025-07-15T00:00:00.000Z'),
    ],
    activeTerm: term('t6-3', 'Term 3', 'Sep-Nov 2025', '2025-07-15T00:00:00.000Z'),
    updatedAt: '2026-05-01T10:00:00.000Z',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm13',
        name: 'Blake Foster',
        status: 'graduated',
        intro:
          'Developed facilitation and inclusive community practices through hands-on work with LF mentors.',
      },
      {
        id: 'm14',
        name: 'Cameron Ellis',
        status: 'graduated',
        intro:
          'Learned open source governance and community leadership by supporting contributor programs.',
      },
      {
        id: 'm15',
        name: 'Devon Price',
        status: 'graduated',
        intro:
          'Built skills in community operations, facilitation, and helping new contributors feel welcome.',
      },
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
    terms: [term('t7-1', 'Term 3', 'Sep-Nov 2026', '2026-07-01T00:00:00.000Z')],
    activeTerm: term('t7-1', 'Term 3', 'Sep-Nov 2026', '2026-07-01T00:00:00.000Z'),
    updatedAt: '2026-08-12T10:00:00.000Z',
    repositoryUrl: 'https://github.com/prometheus',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm16',
        name: 'Amir Haddad',
        status: 'active',
        intro:
          'Extending Prometheus exporters and Grafana dashboards while learning observability patterns with mentors.',
      },
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
      term('t8-1', 'Term 1', 'Jan-Mar 2026', '2025-12-01T00:00:00.000Z'),
      term('t8-2', 'Term 2', 'Apr-Jun 2026', '2026-03-01T00:00:00.000Z'),
    ],
    activeTerm: term('t8-2', 'Term 2', 'Apr-Jun 2026', '2026-03-01T00:00:00.000Z'),
    updatedAt: '2026-07-28T14:00:00.000Z',
    repositoryUrl: 'https://github.com/rust-lang',
    crowdfundingInitiativeId: '9b4080d9-701a-4513-85e6-a162beb3773a',
    mentees: [
      {
        id: 'm17',
        name: 'Lucia Romano',
        status: 'active',
        intro:
          'Learning safe systems programming by contributing to Rust crates used across Linux Foundation projects.',
      },
      {
        id: 'm18',
        name: 'Ben Walsh',
        status: 'graduated',
        intro:
          'Practiced Rust testing and contribution workflows with mentors focused on systems programming.',
      },
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
