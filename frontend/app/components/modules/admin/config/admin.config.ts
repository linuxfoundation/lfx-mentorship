// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { Tab } from '~/components/uikit/tabs/types/tab.types';
import type { TagStyle } from '~/components/uikit/tag/types/tag.types';
import { MENTOR_REGISTER_POLICY_LINKS } from '~/components/modules/mentor-register/config/mentor-register.config';
import type {
  AdminEnrollForm,
  AdminEnrollStep,
  AdminPrerequisite,
  AdminProgram,
  AdminProgramStatus,
  AdminProgramTerm,
} from '~/types/admin.types';
import { plainTextLength } from '~/utils/html-text';

export const ADMIN_TITLE = 'Admin';

export const ADMIN_SUBTITLE =
  'Enroll a mentorship program for a project you maintain. The LFX team reviews every enrollment before it opens for applications.';

export const ADMIN_MY_PROGRAMS_HEADING = 'Programs for which I am an Administrator';

export function adminMyProgramsSubcopy(count: number): string {
  return `You administer ${count} program${count === 1 ? '' : 's'}. Administrators edit program details, invite mentors and control visibility.`;
}

export const ADMIN_STATUS_FILTER_PLACEHOLDER = 'Filter By Status';

export const ADMIN_PROGRAM_STATUS_CONFIG: Record<
  AdminProgramStatus,
  { label: string; variation: TagStyle }
> = {
  published: { label: 'Published', variation: 'positive' },
  'pending-review': { label: 'Pending Review', variation: 'warning' },
};

export const ADMIN_STATUS_FILTER_OPTIONS: Array<{ value: string; label: string }> = [
  { value: 'all', label: 'All statuses' },
  { value: 'published', label: 'Published' },
  { value: 'pending-review', label: 'Pending Review' },
];

export const ADMIN_MOCK_PROGRAMS: AdminProgram[] = [
  {
    id: 'ap-1',
    name: 'GridFlow: Time-Series Ingestion Pipeline',
    foundationName: 'LF Energy',
    termLabel: 'Term 3 - 2026',
    status: 'published',
    stats: {
      mentors: 4,
      currentMentees: 6,
      graduatedMentees: 18,
      fundingToDateCents: 900_000,
    },
  },
  {
    id: 'ap-2',
    name: 'Apicurio Registry: Prompt Template Playground',
    foundationName: 'CNCF',
    termLabel: 'Term 3 - 2026',
    status: 'published',
    stats: {
      mentors: 2,
      currentMentees: 4,
      graduatedMentees: 12,
      fundingToDateCents: 1_200_000,
    },
  },
  {
    id: 'ap-3',
    name: 'Kubernetes Contributors',
    foundationName: 'CNCF',
    termLabel: 'Term 2 - 2026',
    status: 'pending-review',
    stats: {
      mentors: 3,
      currentMentees: 0,
      graduatedMentees: 9,
      fundingToDateCents: 0,
    },
  },
];

export function adminTabItems(programCount: number): Tab[] {
  return [
    { value: 'my-programs', label: `My Programs ${programCount}` },
    { value: 'enroll', label: 'Enroll a Program' },
  ];
}

export const ADMIN_ENROLL_STEP_LABELS: Record<AdminEnrollStep, string> = {
  details: 'Program Details',
  setup: 'Program Setup',
  prerequisites: 'Prerequisites',
};

export const ADMIN_ENROLL_STEPS_ORDER: AdminEnrollStep[] = [
  'details',
  'setup',
  'prerequisites',
];

export const ADMIN_POLICY_LINKS = MENTOR_REGISTER_POLICY_LINKS;

export const ADMIN_IMPORT_PROGRAM_OPTIONS = ADMIN_MOCK_PROGRAMS.map((program) => ({
  value: program.id,
  label: program.name,
}));

export const ADMIN_PROJECT_OPTIONS = [
  { value: 'proj-gridflow', label: 'GridFlow' },
  { value: 'proj-apicurio', label: 'Apicurio Registry' },
  { value: 'proj-k8s', label: 'Kubernetes' },
] as const;

export const ADMIN_NAME_MAX = 100;
export const ADMIN_DESCRIPTION_MAX = 3000;

export const ADMIN_LOGO_ACCEPT = '.jpg,.jpeg,.png,.svg,image/jpeg,image/png,image/svg+xml';
export const ADMIN_LOGO_MAX_BYTES = 2 * 1024 * 1024;
export const ADMIN_LOGO_HELPER = 'JPG, PNG, SVG · 420px × 420px · Max 2 MB';

export const ADMIN_DEFAULT_PREREQUISITES: AdminPrerequisite[] = [
  {
    id: 'prereq-resume',
    name: 'Resume',
    description: 'Upload a current resume or CV.',
    required: false,
  },
  {
    id: 'prereq-cover',
    name: 'Cover Letter',
    description: 'Explain why you want to join this program.',
    required: false,
  },
  {
    id: 'prereq-school',
    name: 'School Enrollment Verification',
    description: 'Proof of current school enrollment, if applicable.',
    required: false,
  },
  {
    id: 'prereq-permission',
    name: 'Participation permission from school or employer',
    description: 'Written permission if required by your institution.',
    required: false,
  },
  {
    id: 'prereq-coding',
    name: 'Coding Challenge',
    description: 'Link to the coding challenge applicants should complete.',
    required: false,
    urlValue: '',
    urlPlaceholder: 'https://www.github.com/...',
  },
];

export const ADMIN_DEFAULT_TERM: AdminProgramTerm = {
  id: 'term-3-2026',
  name: 'Term 3 - 2026',
  startsLabel: 'September 2026',
  endsLabel: 'November 2026',
};

function clonePrerequisites(items: AdminPrerequisite[] = ADMIN_DEFAULT_PREREQUISITES): AdminPrerequisite[] {
  return items.map((item) => ({ ...item }));
}

export function createEmptyAdminEnrollForm(): AdminEnrollForm {
  return {
    importProgramId: '',
    name: '',
    projectId: '',
    technologies: [],
    description: '',
    repositoryUrl: '',
    websiteUrl: '',
    ciiProjectId: '',
    codeOfConductUrl: '',
    logoFileName: '',
    logoPreviewUrl: '',
    skills: [],
    terms: [{ ...ADMIN_DEFAULT_TERM }],
    prerequisites: clonePrerequisites(),
    termsAccepted: false,
  };
}

type ImportedProgramSource = Omit<
  AdminEnrollForm,
  'importProgramId' | 'termsAccepted' | 'logoPreviewUrl'
>;

const ADMIN_IMPORT_PROGRAM_DETAILS: Record<string, ImportedProgramSource> = {
  'ap-1': {
    name: 'GridFlow: Time-Series Ingestion Pipeline',
    projectId: 'proj-gridflow',
    technologies: ['GO', 'Kubernetes', 'GraphQL'],
    description:
      '<p>Build a time-series ingestion pipeline for grid telemetry, including storage, alerting, and contributor onboarding.</p>',
    repositoryUrl: 'https://github.com/lfenergy/gridflow',
    websiteUrl: 'https://lfenergy.org',
    ciiProjectId: '1842',
    codeOfConductUrl: 'https://www.contributor-covenant.org/version/2/1/code_of_conduct/',
    logoFileName: 'gridflow-logo.png',
    skills: ['GO', 'Kubernetes'],
    terms: [{ ...ADMIN_DEFAULT_TERM, name: 'Term 3 - 2026' }],
    prerequisites: clonePrerequisites().map((item, index) => ({
      ...item,
      required: index === 0,
    })),
  },
  'ap-2': {
    name: 'Apicurio Registry: Prompt Template Playground',
    projectId: 'proj-apicurio',
    technologies: ['GO', 'React', 'API'],
    description:
      '<p>Improve the Apicurio Registry prompt-template playground for schema discovery, authoring, and contributor workflows.</p>',
    repositoryUrl: 'https://github.com/Apicurio/apicurio-registry',
    websiteUrl: 'https://www.apicur.io/',
    ciiProjectId: '2104',
    codeOfConductUrl: 'https://github.com/Apicurio/apicurio-registry/blob/main/CODE_OF_CONDUCT.md',
    logoFileName: 'apicurio-logo.png',
    skills: ['GO', 'React', 'API'],
    terms: [{ ...ADMIN_DEFAULT_TERM, name: 'Term 3 - 2026' }],
    prerequisites: clonePrerequisites().map((item) => ({
      ...item,
      required: item.id === 'prereq-resume' || item.id === 'prereq-cover',
    })),
  },
  'ap-3': {
    name: 'Kubernetes Contributors',
    projectId: 'proj-k8s',
    technologies: ['GO', 'Kubernetes', 'Git'],
    description:
      '<p>Help new contributors learn Kubernetes contribution workflows, SIGs, and community practices.</p>',
    repositoryUrl: 'https://github.com/kubernetes/community',
    websiteUrl: 'https://kubernetes.io',
    ciiProjectId: '36',
    codeOfConductUrl: 'https://github.com/kubernetes/community/blob/master/code-of-conduct.md',
    logoFileName: 'kubernetes-logo.png',
    skills: ['GO', 'Kubernetes'],
    terms: [
      {
        id: 'term-2-2026',
        name: 'Term 2 - 2026',
        startsLabel: 'April 2026',
        endsLabel: 'June 2026',
      },
    ],
    prerequisites: clonePrerequisites().map((item) => ({
      ...item,
      required: item.id === 'prereq-resume',
    })),
  },
};

export function formFromImportedProgram(importProgramId: string): AdminEnrollForm {
  if (!importProgramId) {
    return createEmptyAdminEnrollForm();
  }

  const source = ADMIN_IMPORT_PROGRAM_DETAILS[importProgramId];
  if (!source) {
    return { ...createEmptyAdminEnrollForm(), importProgramId };
  }

  return {
    importProgramId,
    name: source.name,
    projectId: source.projectId,
    technologies: [...source.technologies],
    description: source.description,
    repositoryUrl: source.repositoryUrl,
    websiteUrl: source.websiteUrl,
    ciiProjectId: source.ciiProjectId,
    codeOfConductUrl: source.codeOfConductUrl,
    logoFileName: source.logoFileName,
    logoPreviewUrl: '',
    skills: [...source.skills],
    terms: source.terms.map((term) => ({ ...term })),
    prerequisites: clonePrerequisites(source.prerequisites),
    termsAccepted: false,
  };
}

export type AdminEnrollFieldErrors = Partial<Record<string, string>>;

function isBlank(value: string): boolean {
  return !value.trim();
}

export function getAdminEnrollStepErrors(
  step: AdminEnrollStep,
  form: AdminEnrollForm,
): AdminEnrollFieldErrors {
  if (step === 'details') {
    const errors: AdminEnrollFieldErrors = {};
    if (isBlank(form.name)) errors.name = 'Program name is required.';
    if (isBlank(form.projectId)) errors.projectId = 'Select a Linux Foundation project.';
    if (!form.technologies.length) errors.technologies = 'Add at least one technology.';
    if (plainTextLength(form.description) === 0) {
      errors.description = 'Program description is required.';
    }
    if (isBlank(form.repositoryUrl)) errors.repositoryUrl = 'Repository URL is required.';
    if (isBlank(form.logoFileName)) errors.logoFileName = 'Program logo is required.';
    return errors;
  }

  if (step === 'setup') {
    const errors: AdminEnrollFieldErrors = {};
    if (!form.skills.length) errors.skills = 'Add at least one skill.';
    if (!form.terms.length) errors.terms = 'Add at least one program term.';
    return errors;
  }

  const errors: AdminEnrollFieldErrors = {};
  if (!form.prerequisites.some((item) => item.required)) {
    errors.prerequisites = 'Mark at least one prerequisite as required.';
  }
  if (!form.termsAccepted) {
    errors.termsAccepted = 'Please accept the terms and conditions.';
  }
  return errors;
}

export function isAdminEnrollStepValid(step: AdminEnrollStep, form: AdminEnrollForm): boolean {
  return Object.keys(getAdminEnrollStepErrors(step, form)).length === 0;
}

export const ADMIN_DETAILS_INTRO =
  'Tell us about the mentorship program you want to enroll on LFX Mentorship.';

export const ADMIN_SETUP_INTRO =
  'Define the skills mentees need and the term schedule for this program.';

export const ADMIN_SETUP_SKILLS_HELPER =
  'List skills that help match the right mentees. You can invite mentors after enrollment is approved.';

export const ADMIN_SETUP_MENTOR_INFO =
  'After your program is approved, you can invite mentors from the program dashboard.';

export const ADMIN_SETUP_TERMS_HELPER =
  'Add the mentorship terms you plan to run. Applicants apply to a specific term.';

export const ADMIN_PREREQ_INTRO =
  'Select which application materials are required. You can add custom prerequisites if needed.';

export const ADMIN_TERMS_INTRO =
  'Before you submit your program enrollment to the LFX Platform, review and accept the terms and conditions below.';
