// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type {
  EnrollProgramBenefit,
  EnrollProgramGuidanceItem,
  EnrollProgramStep,
} from '~/types/enroll-program.types';

export const ENROLL_PROGRAM_BADGE = 'For maintainers';

export const ENROLL_PROGRAM_HERO_TITLE = 'Offer a mentorship program for your project.';

export const ENROLL_PROGRAM_HERO_SUBTITLE_PARTS: Array<{ text: string; bold?: boolean }> = [
  {
    text: 'Maintainers of Linux Foundation projects can enroll a program and bring on paid, mentored contributors for a ',
  },
  { text: 'multi-week term', bold: true },
  {
    text: '. LFX handles applications, stipends, evaluations and compliance. You scope the work and review it.',
  },
];

export const ENROLL_PROGRAM_CTA_LABEL = 'Enroll a Program in LFX';

export const ENROLL_PROGRAM_CTA_HELPER =
  'Opens the Admin area of LFX. You will need to be signed in as a Program Admin.';

export const ENROLL_PROGRAM_WHY_TITLE = 'Why Offer a Program';

export const ENROLL_PROGRAM_WHY_SUBTITLE = 'What projects get out of running a mentorship term.';

export const ENROLL_PROGRAM_BENEFITS: EnrollProgramBenefit[] = [
  {
    icon: 'code-branch',
    title: 'Grow your contributor base',
    description:
      'Most graduated mentees keep contributing after the term ends, often becoming reviewers and maintainers.',
  },
  {
    icon: 'list-check',
    title: 'Move work you keep deferring',
    description:
      'Programs are well suited to scoped work that never reaches the top of the backlog: tooling, tests, docs, instrumentation.',
  },
  {
    icon: 'sack-dollar',
    title: 'Funding you control',
    description:
      'Where a program is funded, the stipend comes from project funds, a foundation pool or a direct sponsor, so the work is not unpaid labour.',
  },
  {
    icon: 'shield-halved',
    title: 'Program administration handled',
    description:
      'LFX runs applications, eligibility checks, stipend payments, evaluations and compliance.',
  },
  {
    icon: 'users',
    title: 'A wider pool of candidates',
    description:
      'Your program is listed alongside every other LFX program and reaches applicants from outside your existing community.',
  },
  {
    icon: 'chart-line',
    title: 'A record you can point to',
    description:
      'Every term keeps its mentees, approved work and outcomes, so you can show what the project has produced.',
  },
];

export const ENROLL_PROGRAM_HOW_TITLE = 'How to Enroll';

export const ENROLL_PROGRAM_HOW_SUBTITLE =
  'Enrollment is a three-step form in the LFX Admin area. Here is what each step asks for, so you can gather it before you start.';

export const ENROLL_PROGRAM_STEPS: EnrollProgramStep[] = [
  {
    step: 1,
    title: 'Program Details',
    description:
      'What the program is and which project it belongs to. You can import details from a program you have run before.',
    checklist: [
      'Program name',
      'Program description',
      'Code of Conduct URL',
      'Linux Foundation project',
      'Repository and website URLs',
      'Program logo',
      'Technologies',
      'CII Best Practices project ID',
    ],
  },
  {
    step: 2,
    title: 'Program Setup',
    description:
      'Who you are looking for and when the program runs. Mentors are invited later, once the program is approved.',
    checklist: [
      'Required and desirable skills',
      'Custom term dates',
      'Non-technical interest areas',
      'Program terms',
    ],
  },
  {
    step: 3,
    title: 'Prerequisites',
    description: 'What candidates must complete to qualify, plus the platform terms.',
    checklist: [
      'Resume',
      'Participation permission',
      'Terms and conditions',
      'Cover letter topics',
      'Coding challenge',
      'School enrollment verification',
      'Custom prerequisites',
    ],
  },
];

export const ENROLL_PROGRAM_AFTER_TITLE = 'After You Submit';

export const ENROLL_PROGRAM_AFTER_ITEMS: EnrollProgramGuidanceItem[] = [
  {
    text: 'The LFX Mentorship team reviews your enrollment (usually within 2 business days).',
  },
  {
    text: 'Once approved, you invite mentors from the Mentors tab.',
  },
  {
    text: 'The program opens for applications, and applicants appear in your workspace.',
  },
];

export const ENROLL_PROGRAM_BEFORE_TITLE = 'Before You Start';

export const ENROLL_PROGRAM_BEFORE_ITEMS: EnrollProgramGuidanceItem[] = [
  { text: 'Scope the work for a single term.' },
  { text: 'Name at least two mentors.' },
  { text: 'Confirm funding before enrolling (unfunded programs cannot open).' },
];

export const ENROLL_PROGRAM_BOTTOM_CTA_COPY =
  'Ready to enroll your project? Enrollment happens in LFX, where you can also save a draft and come back to it.';
