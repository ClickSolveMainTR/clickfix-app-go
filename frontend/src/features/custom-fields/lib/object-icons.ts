import {
  Box,
  Briefcase,
  Building2,
  Car,
  FileText,
  Package,
  ShoppingCart,
  Tag,
  Ticket,
  Truck,
  Wrench,
  type LucideIcon,
} from "lucide-react";

/** Icons offered for custom object types (stored by name). */
export const OBJECT_ICONS: Record<string, LucideIcon> = {
  box: Box,
  package: Package,
  building: Building2,
  briefcase: Briefcase,
  car: Car,
  truck: Truck,
  wrench: Wrench,
  ticket: Ticket,
  tag: Tag,
  cart: ShoppingCart,
  file: FileText,
};

export function objectIcon(name?: string | null): LucideIcon {
  return (name && OBJECT_ICONS[name]) || Box;
}
