// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Mentee } from '../../app/types/mentee.types';
import type { Mentor } from '../../app/types/mentor.types';

export const MOCK_MENTEES: Mentee[] = [
  {
    id: 'me-1',
    name: 'Hana Suzuki',
    introduction:
      'Backend engineer focused on observability and distributed systems. Contributing to Thanos query-path improvements and mentoring docs for first-time contributors.',
    skills: ['GO', 'Monitoring', 'Kubernetes', 'API'],
    status: 'graduated',
    sinceLabel: 'Since Aug. 2023',
    joinedAt: '2023-08-01T00:00:00.000Z',
    program: { id: 'p-thanos', name: 'Thanos', foundationLabel: 'CNCF' },
    mentors: [
      { id: 'mo-1', name: 'Dana Okafor' },
      { id: 'mo-2', name: 'Amina Reyes' },
    ],
  },
  {
    id: 'me-2',
    name: 'Diego Alvarez',
    introduction:
      'Full-stack contributor building developer tooling. Currently improving React dashboards and API ergonomics for CloudEvents tooling.',
    skills: ['React', 'TypeScript', 'Node.js'],
    status: 'active',
    sinceLabel: 'Since Jan. 2024',
    joinedAt: '2024-01-15T00:00:00.000Z',
    program: { id: 'p-cloudevents', name: 'CloudEvents', foundationLabel: 'CNCF' },
    mentors: [{ id: 'mo-3', name: 'Jordan Blake' }],
  },
  {
    id: 'me-3',
    name: 'Imani Okonkwo',
    introduction:
      'Security-minded mentee hardening CI pipelines and supply-chain checks. Exploring Rust and OpenSSF best practices with mentors.',
    skills: ['Rust', 'Security', 'Continuous integration'],
    status: 'active',
    sinceLabel: 'Since Mar. 2024',
    joinedAt: '2024-03-01T00:00:00.000Z',
    program: { id: 'p-openssf', name: 'OpenSSF Scorecard', foundationLabel: 'OpenSSF' },
    mentors: [
      { id: 'mo-4', name: 'Priya Shah' },
      { id: 'mo-1', name: 'Dana Okafor' },
    ],
  },
  {
    id: 'me-4',
    name: 'Noah Berg',
    introduction:
      'Docs and DX mentee rewriting onboarding guides and examples so new contributors can land their first PR faster.',
    skills: ['Markdown', 'Documentation', 'Vue.js'],
    status: 'graduated',
    sinceLabel: 'Since Sep. 2022',
    joinedAt: '2022-09-10T00:00:00.000Z',
    program: { id: 'p-lfdx', name: 'LF Developer Experience', foundationLabel: 'LF' },
    mentors: [{ id: 'mo-5', name: 'Avery Kim' }],
  },
  {
    id: 'me-5',
    name: 'Sofia Mendes',
    introduction:
      'Cloud-native networking mentee working on Cilium datapath tests and community office hours content.',
    skills: ['Linux', 'Software-defined networking', 'GO'],
    status: 'active',
    sinceLabel: 'Since May 2024',
    joinedAt: '2024-05-01T00:00:00.000Z',
    program: { id: 'p-cilium', name: 'Cilium', foundationLabel: 'CNCF' },
    mentors: [
      { id: 'mo-6', name: 'Chris Okonkwo' },
      { id: 'mo-2', name: 'Amina Reyes' },
      { id: 'mo-3', name: 'Jordan Blake' },
    ],
  },
  {
    id: 'me-6',
    name: 'Kenji Watanabe',
    introduction:
      'AI/ML mentee improving dataset pipelines and model evaluation docs for LF AI & Data programs.',
    skills: ['Python', 'Machine Learning', 'Tensorflow'],
    status: 'graduated',
    sinceLabel: 'Since Feb. 2023',
    joinedAt: '2023-02-20T00:00:00.000Z',
    program: { id: 'p-lfai', name: 'LF AI Model Garden', foundationLabel: 'LF AI' },
    mentors: [{ id: 'mo-7', name: 'Elena Volkov' }],
  },
  {
    id: 'me-7',
    name: 'Maya Patel',
    introduction:
      'Frontend mentee shipping accessible UI patterns and Storybook coverage for mentorship dashboards.',
    skills: ['Vue.js', 'CSS', 'TypeScript'],
    status: 'active',
    sinceLabel: 'Since Jun. 2024',
    joinedAt: '2024-06-12T00:00:00.000Z',
    program: { id: 'p-lfdx', name: 'LF Developer Experience', foundationLabel: 'LF' },
    mentors: [{ id: 'mo-5', name: 'Avery Kim' }],
  },
  {
    id: 'me-8',
    name: 'Omar Hassan',
    introduction:
      'Infrastructure mentee automating Kind-based test clusters and improving contributor local-dev scripts.',
    skills: ['Kubernetes', 'Bash', 'Docker', 'Git'],
    status: 'active',
    sinceLabel: 'Since Apr. 2024',
    joinedAt: '2024-04-08T00:00:00.000Z',
    program: { id: 'p-kubernetes', name: 'Kubernetes', foundationLabel: 'CNCF' },
    mentors: [
      { id: 'mo-6', name: 'Chris Okonkwo' },
      { id: 'mo-4', name: 'Priya Shah' },
    ],
  },
];

export const MOCK_MENTORS: Mentor[] = [
  {
    id: 'mo-1',
    name: 'Dana Okafor',
    bio: 'Staff engineer mentoring on Golang services and observability. Prefers mentees who enjoy debugging production systems and writing clear RFCs.',
    skills: ['GO', 'Kubernetes', 'Monitoring'],
    sinceLabel: 'Since Apr. 2023',
    joinedAt: '2023-04-01T00:00:00.000Z',
    projects: ['Thanos', 'OpenSSF Scorecard'],
  },
  {
    id: 'mo-2',
    name: 'Amina Reyes',
    bio: 'Maintainer focused on networking and eBPF. Happy to mentor contributors who want deep systems work with weekly review cadence.',
    skills: ['Linux', 'Software-defined networking', 'GO'],
    sinceLabel: 'Since Jan. 2022',
    joinedAt: '2022-01-15T00:00:00.000Z',
    projects: ['Cilium', 'Thanos'],
  },
  {
    id: 'mo-3',
    name: 'Jordan Blake',
    bio: 'Product-minded mentor helping mentees ship polished React experiences and strengthen API design instincts.',
    skills: ['React', 'TypeScript', 'Node.js', 'Front end'],
    sinceLabel: 'Since Sep. 2023',
    joinedAt: '2023-09-01T00:00:00.000Z',
    projects: ['CloudEvents', 'Cilium'],
  },
  {
    id: 'mo-4',
    name: 'Priya Shah',
    bio: 'Security mentor covering threat modeling, supply chain, and secure defaults. Looking for mentees curious about OpenSSF tooling.',
    skills: ['Security', 'Rust', 'Continuous integration'],
    sinceLabel: 'Since Mar. 2021',
    joinedAt: '2021-03-10T00:00:00.000Z',
    projects: ['OpenSSF Scorecard', 'Kubernetes'],
  },
  {
    id: 'mo-5',
    name: 'Avery Kim',
    bio: 'Docs lead mentoring on technical writing, information architecture, and contributor onboarding experiences.',
    skills: ['Documentation', 'Markdown', 'Vue.js'],
    sinceLabel: 'Since Jul. 2022',
    joinedAt: '2022-07-01T00:00:00.000Z',
    projects: ['LF Developer Experience'],
  },
  {
    id: 'mo-6',
    name: 'Chris Okonkwo',
    bio: 'Kubernetes maintainer mentoring SIG contribution workflows, CI health, and community collaboration habits.',
    skills: ['Kubernetes', 'GO', 'Git'],
    sinceLabel: 'Since Nov. 2020',
    joinedAt: '2020-11-05T00:00:00.000Z',
    projects: ['Kubernetes', 'Cilium'],
  },
  {
    id: 'mo-7',
    name: 'Elena Volkov',
    bio: 'ML platform mentor guiding mentees through dataset quality, evaluation harnesses, and reproducible training pipelines.',
    skills: ['Python', 'Machine Learning', 'Tensorflow'],
    sinceLabel: 'Since May 2023',
    joinedAt: '2023-05-20T00:00:00.000Z',
    projects: ['LF AI Model Garden'],
  },
  {
    id: 'mo-8',
    name: 'Sam Chen',
    bio: 'Frontend systems mentor focused on accessibility, design tokens, and sustainable component libraries.',
    skills: ['Vue.js', 'CSS', 'TypeScript'],
    sinceLabel: 'Since Feb. 2024',
    joinedAt: '2024-02-01T00:00:00.000Z',
    projects: ['LF Developer Experience', 'CloudEvents'],
  },
];
