import { useQuery } from '@tanstack/react-query';
import { getPerson } from '../api/persons';
import { personKey } from './useFamilyGraph';

/** The full record of one person (bio and residence are not in the graph). Server state, cached by id. */
export function usePersonDetail(id: number) {
  return useQuery({ queryKey: personKey(id), queryFn: () => getPerson(id) });
}
