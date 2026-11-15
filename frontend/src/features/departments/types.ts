import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Department = Schemas["Department"];
export type DepartmentInput = Schemas["DepartmentInput"];
export type DepartmentPage = Schemas["DepartmentPage"];
export type DepartmentRef = Schemas["DepartmentRef"];
export type DepartmentShare = Schemas["DepartmentShare"];
export type DepartmentShareInput = Schemas["DepartmentShareInput"];
export type ConversationDepartment = Schemas["ConversationDepartment"];
export type DepartmentTransfer = Schemas["DepartmentTransfer"];
export type DepartmentTransferInput = Schemas["DepartmentTransferInput"];
export type ShareKind = DepartmentShareInput["resource_kind"];

/** List query of `GET /v1/tenant/departments`. */
export type ListDepartmentsParams = {
  limit: number;
  offset: number;
  q?: string;
  brand?: string;
  /** Only the caller's departments (through their teams). */
  mine?: boolean;
};
