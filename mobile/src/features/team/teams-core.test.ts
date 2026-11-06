import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addSkills,
  defaultWeekDays,
  formErrors,
  formOf,
  hoursSummary,
  matchesWords,
  personName,
  sameForm,
  serverErrors,
  timeZones,
  toRequest,
} from './teams-core.ts';

const team = {
  name: 'Destek',
  description: 'Birinci seviye',
  color: '#2563EB',
  skills: ['faturalama'],
  timezone: 'Europe/Istanbul',
  working_hours: {
    days: defaultWeekDays().map((d) => (d.day === 'sat' ? { ...d, open: true, start: '10:00', end: '14:00' } : d)),
  },
};

test('a new team follows the organization hours', () => {
  const form = formOf();
  assert.equal(form.inherit_hours, true);
  assert.deepEqual(toRequest({ ...form, name: '  Satış ' }), {
    name: 'Satış',
    description: '',
    color: '',
    skills: [],
    inherit_hours: true,
  });
});

test('own hours and time zone round-trip', () => {
  const form = formOf(team);
  assert.equal(form.inherit_hours, false);
  const req = toRequest({ ...form, color: '#2563eb' });
  assert.equal(req.color, '#2563EB');
  assert.equal(req.timezone, 'Europe/Istanbul');
  assert.equal(req.working_hours?.days.length, 7);
  assert.equal(req.working_hours?.days[5]?.start, '10:00');
  assert.equal('inherit_hours' in req, false);
});

test('an empty time zone is sent, so a patch clears the old one', () => {
  const req = toRequest({ ...formOf(team), timezone: '' });
  assert.equal(req.timezone, '');
});

test('a closed day never blocks the form and goes out with valid hours', () => {
  const form = formOf(team);
  form.days[6] = { day: 'sun', open: false, start: '9', end: '' };
  assert.deepEqual(formErrors(form), {});
  assert.deepEqual(toRequest(form).working_hours?.days[6], { day: 'sun', open: false, start: '09:00', end: '18:00' });
  // Valid hours of a closed day are kept for when it opens again.
  form.days[6] = { day: 'sun', open: false, start: '10:00', end: '12:00' };
  assert.deepEqual(toRequest(form).working_hours?.days[6], { day: 'sun', open: false, start: '10:00', end: '12:00' });
});

test('serverErrors translates by code, never by the API message', () => {
  assert.deepEqual(
    serverErrors([
      { field: 'name', code: 'taken' },
      { field: 'skills.1', code: 'invalid' },
      { field: 'skills', code: 'too_many' },
      { field: 'working_hours.days.2.start', code: 'invalid' },
      { field: 'working_hours.days.2.end', code: 'invalid' },
      { field: 'timezone', code: 'invalid' },
      { field: 'color', code: 'brand_new_code' },
      { code: 'invalid' },
    ]),
    {
      name: 'name_taken',
      skills: 'skill',
      'days.2.start': 'time',
      'days.2.end': 'end_invalid',
      timezone: 'timezone',
      color: 'server',
    },
  );
  assert.deepEqual(serverErrors(undefined), {});
  assert.deepEqual(serverErrors([{ field: 'name', code: 'required' }]), { name: 'name_required' });
});

test('matchesWords finds a full name and an e-mail', () => {
  const text = 'Ayşe Nur Yılmaz ayse@ornek.com';
  assert.equal(matchesWords(text, 'Ayşe Yılmaz'), true);
  assert.equal(matchesWords(text, 'yılmaz ayşe'), true);
  assert.equal(matchesWords(text, 'ayse@ornek'), true);
  assert.equal(matchesWords(text, 'demir'), false);
  assert.equal(matchesWords('Işık Demir', 'ışık'), true);
  assert.equal(matchesWords(text, ''), true);
});

test('formErrors mirrors the API checks', () => {
  assert.deepEqual(formErrors(formOf(team)), {});
  assert.equal(formErrors({ ...formOf(team), name: '  ' }).name, 'name_required');
  assert.equal(formErrors({ ...formOf(team), name: 'a'.repeat(121) }).name, 'name_too_long');
  assert.equal(formErrors({ ...formOf(team), description: 'a'.repeat(1001) }).description, 'description_too_long');
  assert.equal(formErrors({ ...formOf(team), color: 'mavi' }).color, 'color');
  assert.equal(formErrors({ ...formOf(team), color: '' }).color, undefined);
  const form = formOf(team);
  form.days[0] = { day: 'mon', open: true, start: '18:00', end: '09:00' };
  form.days[1] = { day: 'tue', open: true, start: '9:00', end: '25:00' };
  form.days[6] = { day: 'sun', open: false, start: '18:00', end: '09:00' };
  assert.deepEqual(formErrors(form), {
    'days.0.end': 'end_after_start',
    'days.1.start': 'time',
    'days.1.end': 'time',
  });
  // Hours are not sent when the team follows the organization.
  assert.deepEqual(formErrors({ ...form, inherit_hours: true }), {});
});

test('sameForm ignores what is not sent', () => {
  const form = formOf(team);
  assert.equal(sameForm(form, { ...form, name: ' Destek ' }), true);
  assert.equal(sameForm(form, { ...form, inherit_hours: true }), false);
  assert.equal(sameForm(form, { ...form, skills: ['faturalama', 'iade'] }), false);
});

test('addSkills normalizes, sorts and rejects like the API', () => {
  assert.deepEqual(addSkills(['iade'], ' Faturalama, English ,iade'), { skills: ['english', 'faturalama', 'iade'] });
  // Lower-cased like the API (no Turkish locale): "IT" is "it", not "ıt".
  assert.deepEqual(addSkills([], 'IT'), { skills: ['it'] });
  assert.deepEqual(addSkills([], '!hata'), { skills: [], error: 'skill' });
  assert.deepEqual(addSkills([], 'a'.repeat(41)), { skills: [], error: 'skill' });
  const full = Array.from({ length: 30 }, (_, i) => `beceri ${String(i).padStart(2, '0')}`);
  assert.equal(addSkills(full, 'yeni').error, 'too_many_skills');
  assert.deepEqual(addSkills(full, 'beceri 01'), { skills: full });
});

test('hoursSummary and personName', () => {
  assert.deepEqual(hoursSummary({ working_hours: null }), { kind: 'org' });
  assert.deepEqual(hoursSummary(team), { kind: 'own', open: 6 });
  assert.equal(personName({ name: 'Ada', surname: 'Yılmaz', email: 'a@b.co' }), 'Ada Yılmaz');
  assert.equal(personName({ name: '', surname: '', email: 'a@b.co' }), 'a@b.co');
});

test('timeZones keeps the team zone listed', () => {
  const zones = timeZones('Mars/Olympus');
  assert.ok(zones.includes('UTC'));
  assert.ok(zones.includes('Europe/Istanbul'));
  assert.ok(zones.includes('Mars/Olympus'));
});
