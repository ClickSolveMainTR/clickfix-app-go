import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import { P, useCan } from '@/core/auth/permissions';
import { useAgents, useTags } from '@/features/inbox/api';

import {
  decodeRule,
  type Names,
  type Page,
  patchRow,
  type RawRule,
  type Rule,
  type RuleExecution,
  toRuleInput,
  type Workflow,
} from './core';

/**
 * Workflows and automation rules (CF-164), the endpoints the panel calls
 * (`frontend/src/features/workflows/services/workflows.service.ts`,
 * `frontend/src/features/automation-rules/services/rules.service.ts`).
 * Read with `tenant.automations.read`; switching one on or off needs
 * `tenant.automations.write`. Create, edit and delete stay on the panel.
 */

const keys = {
  workflows: (org: string) => orgKey(org, 'automations', 'workflows'),
  lastRun: (org: string, uuid: string) => orgKey(org, 'automations', 'workflow-last-run', uuid),
  rules: (org: string) => orgKey(org, 'automations', 'rules'),
  executions: (org: string) => orgKey(org, 'automations', 'rule-executions'),
};

export function useAutomationScope() {
  const org = useOrgSlug();
  const can = useCan();
  return { org, canRead: can(P.automationsRead), canWrite: can(P.automationsWrite) };
}

export function useWorkflows() {
  const { org, canRead } = useAutomationScope();
  return useQuery({
    queryKey: keys.workflows(org),
    enabled: !!org && canRead,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/workflows', { params: { header: orgHeader(), query: { limit: 100, offset: 0 } } })),
  });
}

/** Newest run of a published workflow; a draft never ran. */
export function useWorkflowLastRun(uuid: string, enabled: boolean) {
  const { org, canRead } = useAutomationScope();
  return useQuery({
    queryKey: keys.lastRun(org, uuid),
    enabled: !!org && canRead && enabled && !!uuid,
    staleTime: 30_000,
    queryFn: async () => {
      const page = await unwrap(
        api.GET('/v1/tenant/workflows/{uuid}/runs', {
          params: { header: orgHeader(), path: { uuid }, query: { limit: 1, offset: 0 } },
        }),
      );
      return page.items[0] ?? null;
    },
  });
}

/** The switch moves at once; a refused change puts it back (the screen shows why). */
export function useToggleWorkflow() {
  const qc = useQueryClient();
  const { org } = useAutomationScope();
  const key = keys.workflows(org);
  return useMutation({
    mutationFn: ({ uuid, enabled }: { uuid: string; enabled: boolean }) =>
      unwrap(
        api.POST('/v1/tenant/workflows/{uuid}/enable', {
          params: { header: orgHeader(), path: { uuid } },
          body: { enabled },
        }),
      ),
    onMutate: async ({ uuid, enabled }) => {
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<Page<Workflow>>(key);
      qc.setQueryData<Page<Workflow>>(key, patchRow(prev, uuid, { enabled }));
      return { prev };
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev);
    },
    onSuccess: (row) => qc.setQueryData<Page<Workflow>>(key, (page) => patchRow(page, row.uuid, row)),
  });
}

/** All rules in evaluation order (position, then creation). */
export function useRules() {
  const { org, canRead } = useAutomationScope();
  return useQuery({
    queryKey: keys.rules(org),
    enabled: !!org && canRead,
    queryFn: async (): Promise<Page<Rule>> => {
      // The OpenAPI document types this answer as an opaque envelope.
      const page = (await unwrap(
        api.GET('/v1/tenant/automations', { params: { header: orgHeader(), query: { limit: 100, offset: 0 } } }),
      )) as Page<RawRule>;
      return { ...page, items: (page.items ?? []).map(decodeRule) };
    },
  });
}

/** The newest evaluations of every rule (run log, newest first). */
export function useRuleExecutions() {
  const { org, canRead } = useAutomationScope();
  return useQuery({
    queryKey: keys.executions(org),
    enabled: !!org && canRead,
    staleTime: 30_000,
    queryFn: async () => {
      const page = (await unwrap(
        api.GET('/v1/tenant/automations/executions', {
          params: { header: orgHeader(), query: { limit: 100, offset: 0 } },
        }),
      )) as Page<RuleExecution>;
      return page.items ?? [];
    },
  });
}

/**
 * Rules have no enable endpoint: like the panel's list switch, the rule is
 * saved again with only `enabled` changed (`PUT`, full replacement).
 */
export function useToggleRule() {
  const qc = useQueryClient();
  const { org } = useAutomationScope();
  const key = keys.rules(org);
  return useMutation({
    mutationFn: ({ rule, enabled }: { rule: Rule; enabled: boolean }) =>
      unwrap(
        api.PUT('/v1/tenant/automations/{uuid}', {
          params: { header: orgHeader(), path: { uuid: rule.uuid } },
          body: { ...toRuleInput(rule), enabled },
        }),
      ),
    onMutate: async ({ rule, enabled }) => {
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<Page<Rule>>(key);
      qc.setQueryData<Page<Rule>>(key, patchRow(prev, rule.uuid, { enabled }));
      return { prev };
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });
}

/**
 * Names for the tags and agents a condition or an action points at. Both
 * lists have their own permission; without it the summary shows the action
 * without a name.
 */
export function useTargetNames(): Names {
  const can = useCan();
  const allowed = can(P.conversationsRead);
  const tags = useTags(allowed);
  const agents = useAgents(allowed);
  return useMemo(() => {
    const names = new Map<string, string>();
    for (const tag of tags.data?.items ?? []) names.set(tag.uuid, tag.name);
    for (const a of agents.data?.items ?? []) names.set(a.uuid, a.name || a.email);
    return names;
  }, [tags.data, agents.data]);
}
