import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Category = Schemas["Category"];
export type CategoryInput = Schemas["CategoryInput"];
export type CategoryList = Schemas["CategoryList"];
export type CategoryAssignment = Schemas["CategoryAssignment"];
export type CategoryAssignmentPage = Schemas["CategoryAssignmentPage"];
export type CategoryAssignmentResult = Schemas["CategoryAssignmentResult"];
export type CategorySubjectType = Schemas["CategorySubjectType"];
export type CategorySource = Schemas["CategorySource"];

/** A category with its children, built from the flat API list. */
export type CategoryNode = Category & { children: CategoryNode[] };
