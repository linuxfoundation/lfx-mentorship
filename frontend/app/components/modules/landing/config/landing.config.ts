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
  'LFX Mentorship pairs contributors with maintainers of the projects that run modern infrastructure. Over a 12-week term you take on real, scoped work, review it with your mentor, and earn a stipend for completing it.';

export const LANDING_HERO_FEATURES: LandingHeroFeature[] = [
  { icon: 'sack-dollar', label: 'Paid stipend each term' },
  { icon: 'calendar', label: '12-week terms, three per year' },
  { icon: 'layer-group', label: '128 programs across 42 foundations' },
];

export const LANDING_HERO_TERM_STATUS = {
  label: 'Term 3 applications open',
  closesLabel: 'Closes Jul 15, 2026',
};

export const LANDING_STATS: LandingStat[] = [
  { value: '128', label: 'Programs Accepting' },
  { value: '2,057', label: 'Mentees Graduated' },
  { value: '$6.1M', label: 'Stipends Paid' },
  { value: '840', label: 'Active Mentors' },
];

export const LANDING_HOW_IT_WORKS_SUBTITLE = 'Five steps from application to graduation.';

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
    text: 'You are 18 or older and legally able to receive a stipend.',
  },
  {
    id: 'e2',
    text: 'You can commit roughly 20 hours a week for the full 12-week term.',
    parts: [
      { text: 'You can commit roughly ' },
      { text: '20 hours a week', bold: true },
      { text: ' for the full 12-week term.' },
    ],
  },
  {
    id: 'e3',
    text: 'You are not concurrently enrolled in another paid mentorship for the same term.',
  },
  {
    id: 'e4',
    text: 'You hold at most one active LFX mentorship at a time.',
  },
];

export const LANDING_ELIGIBILITY_FOOTER =
  'Stipend amounts vary by program and by country of residence. They are paid in three installments over the term.';

export const LANDING_BENEFITS: LandingBenefit[] = [
  {
    icon: 'sack-dollar',
    title: 'Paid, not volunteered',
    description:
      'Every mentorship carries a stipend for the 12-week term, paid in three milestone installments. Amounts vary by program and country of residence.',
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
      'Most programs expect you to learn the domain in the first two weeks. Prior open source work is not required.',
  },
];

export const LANDING_FAQ_SUBTITLE = 'What applicants ask most often before their first term.';

export const LANDING_FAQ_ITEMS: LandingFaqItem[] = [
  {
    question: 'How much is the stipend?',
    answer:
      'It varies by program and by your country of residence. Each program page lists its own amount before you apply.',
  },
  {
    question: 'Who can apply?',
    answer:
      'Anyone 18 or older who can legally receive a stipend and is not enrolled in another paid mentorship for the same term.',
  },
  {
    question: 'What happens at the end?',
    answer:
      'You complete a final evaluation with your mentor. Graduating releases your last stipend installment and is recorded on your profile.',
  },
  {
    question: 'How much time does a term take?',
    answer: 'Around 20 hours a week for 12 weeks. You may hold one active mentorship at a time.',
  },
  {
    question: 'Do I need to be a student?',
    answer: 'No. Some programs ask for school enrollment verification, but many do not.',
  },
  {
    question: 'How many programs can I apply to?',
    answer: 'Up to three per term. Withdrawing an application frees a slot.',
  },
  {
    question: 'How are applicants chosen?',
    answer:
      'Program mentors review each application against the skills they listed and what you wrote in your cover letter.',
  },
];

export const LANDING_CTA_TITLE = '128 programs are accepting applications';

export const LANDING_CTA_SUBTITLE =
  'Browse by foundation, skill or term. You can submit up to three applications per term.';
