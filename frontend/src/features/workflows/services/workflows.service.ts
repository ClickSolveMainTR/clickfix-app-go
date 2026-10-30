import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type Workflow = components["schemas"]["Workflow"];
export type WorkflowInput = components["schemas"]["WorkflowInput"];
export type WorkflowVersion = components["schemas"]["WorkflowVersion"];
export type WorkflowRun = components["schemas"]["WorkflowRun"];
export type WorkflowStep = components["schemas"]["WorkflowStep"];
export type WorkflowTrace = components["schemas"]["WorkflowTrace"];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/**
 * Visual workflows (CF-96). Read with tenant.automations.read, write with
 * tenant.automations.write. Saving only changes the draft (optimistic
 * `revision`); publishing freezes the draft as an immutable version. Save,
 * publish and test are silent so the editor can show the validation message
 * next to the graph instead of a global toast.
 */
export const workflowsService = {
  async list(limit = 50, offset = 0) {
    return unwrap<Page<Workflow>>(
      await apiClient.GET("/v1/tenant/workflows", {
        params: { header: orgHeader(), query: { limit, offset } },
      }),
    );
  },

  async get(uuid: string) {
    return unwrap<Workflow>(
      await apiClient.GET("/v1/tenant/workflows/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async create(body: WorkflowInput) {
    return unwrap<Workflow>(
      await apiClient.POST("/v1/tenant/workflows", {
        params: { header: orgHeader() },
        body,
      }),
      { silent: true },
    );
  },

  async save(uuid: string, body: WorkflowInput) {
    return unwrap<Workflow>(
      await apiClient.PUT("/v1/tenant/workflows/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
      { silent: true },
    );
  },

  async publish(uuid: string, revision: number) {
    return unwrap<Workflow>(
      await apiClient.POST("/v1/tenant/workflows/{uuid}/publish", {
        params: { header: orgHeader(), path: { uuid } },
        body: { revision },
      }),
      { silent: true },
    );
  },

  async enable(uuid: string, enabled: boolean) {
    return unwrap<Workflow>(
      await apiClient.POST("/v1/tenant/workflows/{uuid}/enable", {
        params: { header: orgHeader(), path: { uuid } },
        body: { enabled },
      }),
    );
  },

  async versions(uuid: string) {
    return unwrap<WorkflowVersion[]>(
      await apiClient.GET("/v1/tenant/workflows/{uuid}/versions", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  /** Dry-run of the saved draft against a sample conversation; writes nothing. */
  async test(uuid: string, conversation: string) {
    return unwrap<WorkflowTrace>(
      await apiClient.POST("/v1/tenant/workflows/{uuid}/test", {
        params: { header: orgHeader(), path: { uuid } },
        body: { conversation_uuid: conversation },
      }),
      { silent: true },
    );
  },

  async runs(uuid: string, limit = 20, offset = 0) {
    return unwrap<Page<WorkflowRun>>(
      await apiClient.GET("/v1/tenant/workflows/{uuid}/runs", {
        params: {
          header: orgHeader(),
          path: { uuid },
          query: { limit, offset },
        },
      }),
    );
  },

  /** Run detail with ordered step log and the pinned version graph. */
  async run(run: string) {
    return unwrap<{
      run: WorkflowRun;
      steps: WorkflowStep[];
      version: WorkflowVersion;
    }>(
      await apiClient.GET("/v1/tenant/workflow-runs/{run}", {
        params: { header: orgHeader(), path: { run } },
      }),
      { silent: true },
    );
  },

  /** Imports a routing rule as a published, disabled workflow. */
  async importRouting(rule: string) {
    return unwrap<Workflow>(
      await apiClient.POST("/v1/tenant/workflow-imports/routing", {
        params: { header: orgHeader() },
        body: { routing_rule_uuid: rule },
      }),
    );
  },
};
