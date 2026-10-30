import { ObjectRecordsPage } from "@/features/custom-fields/components/records/object-records-page";

type ObjectRecordsRouteProps = {
  params: Promise<{ typeUuid: string }>;
};

export default async function ObjectRecordsRoute({
  params,
}: ObjectRecordsRouteProps) {
  const { typeUuid } = await params;
  return <ObjectRecordsPage key={typeUuid} typeUuid={typeUuid} />;
}
