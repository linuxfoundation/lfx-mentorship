// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

import { mountSuspended } from '@nuxt/test-utils/runtime';
import { describe, expect, it } from 'vitest';
import LfxCheckbox from './checkbox/checkbox.vue';
import LfxChip from './chip/chip.vue';
import LfxDatepicker from './datepicker/datepicker.vue';
import LfxField from './field/field.vue';
import LfxFieldMessage from './field/field-message.vue';
import LfxFieldMessages from './field/field-messages.vue';
import LfxLinkButton from './link-button/link-button.vue';
import LfxProgressBar from './progress-bar/progress-bar.vue';
import LfxRadio from './radio/radio.vue';
import LfxTextarea from './textarea/textarea.vue';
import LfxToggle from './toggle/toggle.vue';

describe('form controls', () => {
  it.each([
    ['checkbox', LfxCheckbox],
    ['toggle', LfxToggle],
  ])('%s emits its new checked state', async (_name, component) => {
    const wrapper = await mountSuspended(component, {
      props: { modelValue: false },
      slots: { default: () => 'Agree' },
    });
    expect(wrapper.text()).toContain('Agree');
    await wrapper.get('input').setValue(true);
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]]);
  });

  it('shows the checkbox tick only when checked', async () => {
    expect(
      (await mountSuspended(LfxCheckbox, { props: { modelValue: true } }))
        .find('.c-checkbox__check')
        .exists(),
    ).toBe(true);
    expect(
      (await mountSuspended(LfxCheckbox, { props: { modelValue: false } }))
        .find('.c-checkbox__check')
        .exists(),
    ).toBe(false);
  });

  it('emits the radio value when selected, and honours disabled', async () => {
    const wrapper = await mountSuspended(LfxRadio, {
      props: { modelValue: 'a', value: 'b', name: 'choice', size: 'small' },
      slots: { default: () => 'Option B' },
    });
    expect(wrapper.classes()).toContain('c-radio--small');
    await wrapper.get('input').setValue(true);
    expect(wrapper.emitted('update:modelValue')).toEqual([['b']]);

    const disabled = await mountSuspended(LfxRadio, {
      props: { modelValue: 'a', value: 'b', disabled: true },
    });
    expect(disabled.classes()).toContain('c-radio--disabled');
    expect(disabled.get('input').attributes()).toHaveProperty('disabled');
    expect(disabled.find('.c-radio__label').exists()).toBe(false);
  });

  it('emits textarea input and marks it invalid', async () => {
    const wrapper = await mountSuspended(LfxTextarea, {
      props: { modelValue: '', invalid: true, placeholder: 'Why?' },
    });
    expect(wrapper.classes()).toContain('is-invalid');
    expect(wrapper.get('textarea').attributes('placeholder')).toBe('Why?');
    await wrapper.get('textarea').setValue('Because');
    expect(wrapper.emitted('update:modelValue')).toEqual([['Because']]);
  });

  it('marks a required field', async () => {
    const required = await mountSuspended(LfxField, { props: { label: 'Name', required: true } });
    expect(required.text()).toMatch(/^\*\s+Name$/);
    expect((await mountSuspended(LfxField, { props: { label: 'Name' } })).text()).toBe('Name');
  });

  it('exposes setViewDate on the datepicker', async () => {
    const wrapper = await mountSuspended(LfxDatepicker);
    expect(() =>
      (wrapper.vm as unknown as { setViewDate: (d: Date) => void }).setViewDate(
        new Date('2026-03-01'),
      ),
    ).not.toThrow();
  });
});

describe('field messages', () => {
  it('renders a typed message with its icon unless hidden', async () => {
    const wrapper = await mountSuspended(LfxFieldMessage, {
      props: { type: 'error' },
      slots: { default: () => 'Required' },
    });
    expect(wrapper.classes()).toContain('c-field-message--error');
    expect(wrapper.text()).toContain('Required');
    const hidden = await mountSuspended(LfxFieldMessage, {
      props: { type: 'error', hideIcon: true },
    });
    expect(hidden.find('i, svg, .fa-light, .fa-solid').exists()).toBe(false);
  });

  const validation = (validator: string, message: string) => ({
    $errors: [
      { $property: 'name', $validator: validator, $message: message },
      { $property: 'other', $validator: 'x', $message: 'second error' },
    ],
  });

  it('shows only the first validation error', async () => {
    const wrapper = await mountSuspended(LfxFieldMessages, {
      props: { validation: validation('required', 'Value is required') },
    });
    expect(wrapper.text()).toContain('Value is required');
    expect(wrapper.text()).not.toContain('second error');
  });

  it('prefers a custom message and can hide defaults', async () => {
    const custom = await mountSuspended(LfxFieldMessages, {
      props: {
        validation: validation('required', 'Value is required'),
        errorMessages: { required: 'Name is required' },
      },
    });
    expect(custom.text()).toContain('Name is required');
    const hidden = await mountSuspended(LfxFieldMessages, {
      props: { validation: validation('required', 'Value is required'), hideDefault: true },
    });
    expect(hidden.text()).toBe('');
    const none = await mountSuspended(LfxFieldMessages, { props: { validation: { $errors: [] } } });
    expect(none.text()).toBe('');
  });
});

describe('display components', () => {
  it('dismisses a removable chip', async () => {
    const wrapper = await mountSuspended(LfxChip, {
      props: { removable: true, type: 'info', size: 'small' },
      slots: { default: () => 'Go' },
    });
    expect(wrapper.classes()).toEqual(
      expect.arrayContaining(['p-chip-size-small', 'p-chip-type-info']),
    );
    await wrapper.get('[aria-label="Remove"]').trigger('click');
    expect(wrapper.emitted('dismissed')).toHaveLength(1);
    expect(wrapper.find('.p-chip').exists()).toBe(false);
    expect((await mountSuspended(LfxChip)).find('[aria-label="Remove"]').exists()).toBe(false);
  });

  it.each([
    ['left', true],
    ['right', false],
  ] as const)('places the link button icon on the %s', async (iconPosition, iconFirst) => {
    const wrapper = await mountSuspended(LfxLinkButton, {
      props: { label: 'More', icon: 'arrow-right', iconPosition, variant: 'accent' },
    });
    expect(wrapper.classes()).toContain('text-accent-500');
    expect(wrapper.text()).toBe('More');
    const html = wrapper.html();
    expect(html.indexOf('arrow-right') < html.indexOf('More')).toBe(iconFirst);
  });

  it('draws progress segments with widths, colours, and a minimum width', async () => {
    const wrapper = await mountSuspended(LfxProgressBar, {
      props: {
        values: [0.5, 30, 0],
        colors: ['red'],
        minSegmentWidth: 4,
        label: 'Funded',
        tooltips: ['', 'Thirty'],
      },
    });
    const segments = wrapper.findAll('.c-progress-bar__value');
    expect(segments[0]!.attributes('style')).toContain('min-width: 4px');
    expect(segments[0]!.attributes('style')).toContain('background-color: red');
    expect(segments[1]!.attributes('style')).toContain('width: 30%');
    expect(wrapper.get('.c-progress-bar').classes()).toContain('c-progress-bar--interactive');
    expect(wrapper.text()).toContain('Funded');
    expect(wrapper.find('.c-progress-bar__empty').exists()).toBe(true);
  });
});
