// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type {
  LandingBenefit,
  LandingEligibilityItem,
  LandingFaqItem,
  LandingHeroFeature,
  LandingHowItWorksStep,
  LandingStat,
} from '~/types/landing.types';

export const LANDING_GRADUATED_COUNT_LABEL = '2,057 mentees graduated since 2019';

export const LANDING_HERO_AVATARS = [
  'https://i.pravatar.cc/40?img=12',
  'https://i.pravatar.cc/40?img=32',
  'https://i.pravatar.cc/40?img=47',
  'https://i.pravatar.cc/40?img=5',
] as const;

export const LANDING_HERO_TITLE = 'Learn open source by building it.';

export const LANDING_HERO_SUBTITLE =
  'LFX Mentorship pairs contributors with maintainers of the projects that run modern infrastructure. Over the course of a term you take on real, scoped work and review it with your mentor.';

export const LANDING_HERO_FEATURES: LandingHeroFeature[] = [
  { icon: 'sack-dollar', label: 'Many programs offer a stipend' },
  { icon: 'calendar', iconType: 'solid', label: 'Multi-week programs' },
  { icon: 'layer-group', label: '128 programs across 42 foundations' },
];

export const LANDING_STATS: LandingStat[] = [
  { value: '128', label: 'Programs Accepting' },
  { value: '2,057', label: 'Mentees Graduated' },
  { value: '$6.1M', label: 'Stipends Paid' },
  { value: '840', label: 'Active Mentors' },
];


export const LANDING_HOW_IT_WORKS_STEPS: LandingHowItWorksStep[] = [
  {
    step: 1,
    title: 'Build your profile',
    description: 'Add your skills, links and a short introduction. Mentors read this first.',
  },
  {
    step: 2,
    title: 'Apply to programs',
    description: 'Up to three applications per term. Each one is reviewed by the program mentors.',
  },
  {
    step: 3,
    title: 'Get matched',
    description: 'Mentors score applicants on skills fit and intent, then accept for the term.',
  },
  {
    step: 4,
    title: 'Do the work',
    description: 'Twelve weeks of scoped tasks with weekly reviews from your mentor.',
  },
  {
    step: 5,
    title: 'Graduate',
    description: 'Pass the final evaluation and receive your stipend installments.',
  },
];

export const LANDING_ELIGIBILITY_ITEMS: LandingEligibilityItem[] = [
  {
    id: 'e1',
    text: 'You are 18 or older.',
  },
  {
    id: 'e2',
    text: 'You can commit consistent hours for the full term.',
  },
  {
    id: 'e3',
    text: 'You may have up to 3 applications at a time.',
  },
  {
    id: 'e4',
    text: 'You may only enter one Mentorship program in your career.',
  },
];

export const LANDING_ELIGIBILITY_FOOTER =
  'Not every program is funded. Where a stipend is offered, the amount varies by program and by your country of residence. Each program page lists its own stipend.';

export const LANDING_BENEFITS: LandingBenefit[] = [
  {
    icon: 'sack-dollar',
    title: 'Funded work where available',
    description:
      'Many programs carry a stipend, paid in milestone installments. Amounts vary by program and country of residence.',
  },
  {
    icon: 'user-tie',
    title: 'A named mentor',
    description:
      'You work with a maintainer of the project who scopes your tasks and reviews everything you submit.',
  },
  {
    icon: 'code-branch',
    title: 'Real, merged work',
    description:
      'Tasks come from the project backlog, so what you finish ships to the people who depend on it.',
  },
  {
    icon: 'door-open',
    title: 'No experience required',
    description:
      'Most programs expect you to learn the domain as you start. Prior open source work is not required.',
  },
];

export const LANDING_FAQ_ITEMS: LandingFaqItem[] = [
  {
    question: 'Is there a stipend?',
    answer:
      'Not every program is funded. Where a stipend is offered it varies by program and by your country of residence, and the program page says so before you apply.',
  },
  {
    question: 'Who can apply?',
    answer: 'Anyone 18 or older who has not previously completed an LFX mentorship.',
  },
  {
    question: 'What happens at the end?',
    answer:
      'You complete a final evaluation with your mentor. Graduating is recorded on your profile.',
  },
  {
    question: 'How much time does a term take?',
    answer:
      'A significant weekly commitment across a multi-week program. Each program page lists its own term dates.',
  },
  {
    question: 'Do I need to be a student?',
    answer: 'No. Some programs ask for school enrollment verification, but many do not.',
  },
  {
    question: 'How many programs can I apply to?',
    answer:
      'You may have up to three applications at a time. Withdrawing an application frees a slot.',
  },
  {
    question: 'How are applicants chosen?',
    answer:
      'Program mentors review each application against the skills they listed and what you wrote in your cover letter.',
  },
];

export const LANDING_CTA_TITLE = '128 programs are accepting applications';

export const LANDING_CTA_SUBTITLE =
  'Browse by foundation, skill or term. You can have up to three applications at a time.';
