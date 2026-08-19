// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import type { TagStyle } from '~/components/uikit/tag/types/tag.types';
import type {
  MentorProgramRequest,
  MentorRegisterForm,
  MentorRegisterPolicyLink,
  MentorRequestStatus,
} from '~/types/mentor-register.types';
import type { Program } from '~/types/program.types';
import { SKILL_OPTIONS } from '~/config/skills';
import { plainTextLength } from '~/utils/html-text';

export const MENTOR_REGISTER_TITLE = 'Become a Mentor';

export const MENTOR_REGISTER_SUBTITLE =
  'Register as a mentor and request to join the programs you want to support. Fields marked * are required.';

export const MENTOR_REGISTER_PROGRAMS_INTRO =
  'Select the LFX mentorship you would like to join as a mentor, and the program administrator will be notified of your request.';

export const MENTOR_REGISTER_PROGRAMS_HELPER =
  'Selecting a program sends a request to its administrator. You can request more than one.';

export const MENTOR_REGISTER_INTRODUCTION_INTRO =
  'This information is displayed on your mentor profile page. Your name, email and avatar come from your LFX account.';

export const MENTOR_REGISTER_INTRODUCTION_PLACEHOLDER = `What is your current contributor status (i.e., experience in contributing to or maintaining open source projects, open source contributions)?

Why are you interested in volunteering as a mentor?

Tell us something that makes you unique.`;

export const MENTOR_REGISTER_SKILLS_INTRO =
  'What are the skills that you are respected and known for? This helps match you with the right candidates.';

export const MENTOR_REGISTER_LINKS_INTRO =
  'Optional, but candidates often look you up before applying.';

export const MENTOR_REGISTER_TERMS_INTRO =
  'Before you submit your mentor registration to the LFX Platform, review and accept the terms and conditions below.';

export const MENTOR_REGISTER_EXPORT_DISCLAIMER =
  'At this moment we are not accepting applications from a person or entity restricted by U.S. export controls or sanction programs, or a resident of Cuba, Iran, North Korea, Syria, Sudan, Russian Federation or Crimea region of Ukraine.';

export const MENTOR_REGISTER_COMPLIANCE_LEAD =
  'I hereby certify that I am not, and/or the organization I am representing is not:';

export const MENTOR_REGISTER_COMPLIANCE_ITEMS = [
  'located in Cuba, Iran, North Korea, Syria, the Crimea Region of Ukraine, or the Russian-controlled areas of the Donetsk or Luhansk regions of Ukraine;',
  'owned or controlled by, acting for or on behalf of, or an individual or entity that has in the past acted for or on behalf of the Government of Cuba, Iran, North Korea, Syria, or Venezuela; or',
  "listed as a blocked person by the U.S. Department of the Treasury's Office of Foreign Assets Control (OFAC), or directly or indirectly owned 50 percent or more by such a listed person.",
] as const;

export const MENTOR_REGISTER_POLICY_LINKS: MentorRegisterPolicyLink[] = [
  {
    label: 'LFX Platform Use Agreement',
    href: 'https://www.linuxfoundation.org/legal/platform-use-agreement',
  },
  {
    label: 'Service-Specific Use Terms',
    href: 'https://www.linuxfoundation.org/legal/service-specific-terms',
  },
  {
    label: 'Acceptable Use Policy',
    href: 'https://www.linuxfoundation.org/legal/acceptable-use',
  },
  {
    label: 'Privacy Policy',
    href: 'https://www.linuxfoundation.org/privacy',
  },
];

export const MENTOR_REGISTER_RESUME_ACCEPT = '.pdf,.doc,.docx,application/pdf';
export const MENTOR_REGISTER_RESUME_MAX_BYTES = 10 * 1024 * 1024;
export const MENTOR_REGISTER_RESUME_HELPER = 'File type: PDF, .DOC, .DOCX';
export const MENTOR_REGISTER_RESUME_SIZE_HELPER = 'Max size: 10 MB';


export const MENTOR_REQUEST_STATUS_CONFIG: Record<
  MentorRequestStatus,
  { label: string; variation: TagStyle }
> = {
  approved: { label: 'Approved', variation: 'positive' },
  pending: { label: 'Pending', variation: 'warning' },
};

/** Seed rows matching the mock UI while the API is not wired. */
export const MENTOR_REGISTER_SEED_REQUESTS: MentorProgramRequest[] = [
  {
    id: 'req-1',
    programId: '1',
    programName: 'Kubernetes Contributors',
    status: 'approved',
  },
  {
    id: 'req-2',
    programId: '4',
    programName: 'Apicurio Registry: Prompt Template Playground',
    status: 'pending',
  },
];

export function createEmptyMentorRegisterForm(): MentorRegisterForm {
  return {
    introduction: '',
    skills: ['GO', 'Kubernetes'],
    linkedinUrl: '',
    githubUrl: '',
    resumeFileName: '',
    complianceAccepted: false,
    termsAccepted: false,
  };
}

export type MentorRegisterFieldErrors = Partial<Record<string, string>>;

function toEditorHtml(text: string): string {
  const value = text.trim();
  if (!value) return '';
  if (/<[a-z][\s\S]*>/i.test(value)) return value;
  return `<p>${value}</p>`;
}

export function skillsFromProgram(program: Program): string[] {
  const catalog = new Map<string, string>(
    SKILL_OPTIONS.map((skill: string) => [skill.toLowerCase(), skill]),
  );

  return program.skills.flatMap((skill) => {
    const match = catalog.get(skill.toLowerCase());
    return match ? [match] : [];
  });
}

/** Prefills mentor form fields from a selected existing program. */
export function applyProgramToMentorForm(
  form: MentorRegisterForm,
  program: Program,
): Pick<MentorRegisterForm, 'introduction' | 'skills'> {
  const incoming = skillsFromProgram(program);
  const seen = new Set(form.skills.map((skill) => skill.toLowerCase()));
  const skills = [...form.skills];

  for (const skill of incoming) {
    if (seen.has(skill.toLowerCase())) continue;
    skills.push(skill);
    seen.add(skill.toLowerCase());
  }

  const introduction =
    plainTextLength(form.introduction) > 0
      ? form.introduction
      : toEditorHtml(program.description);

  return { introduction, skills };
}

export function getMentorRegisterErrors(
  form: MentorRegisterForm,
  requestCount: number,
): MentorRegisterFieldErrors {
  const errors: MentorRegisterFieldErrors = {};

  if (requestCount < 1) {
    errors.programs = 'Select at least one mentorship program.';
  }
  if (plainTextLength(form.introduction) === 0) {
    errors.introduction = 'Introduction is required.';
  }
  if (!form.skills.length) {
    errors.skills = 'Add at least one skill.';
  }
  if (!form.complianceAccepted) {
    errors.complianceAccepted = 'Please confirm the compliance statement.';
  }
  if (!form.termsAccepted) {
    errors.termsAccepted = 'Please accept the terms and conditions.';
  }

  return errors;
}
