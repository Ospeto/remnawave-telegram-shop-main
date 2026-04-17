import { Plan } from './types';

export interface ResolvedPlanReference {
    legacyIndex: number | null;
    plan: Plan | undefined;
}

export function isPlanArchived(plan: Plan): boolean {
    return plan.active === false;
}

export function getVisiblePlans(plans: Plan[]): Plan[] {
    return plans.filter((plan) => !isPlanArchived(plan));
}

export function resolvePlanReference(plans: Plan[], reference: string | undefined | null): ResolvedPlanReference {
    const normalizedReference = reference?.trim();
    if (!normalizedReference) {
        return { legacyIndex: null, plan: undefined };
    }

    const directMatch = plans.find((plan) => String(plan.id) === normalizedReference);
    if (directMatch) {
        return { legacyIndex: null, plan: directMatch };
    }

	const legacyIndex = Number(normalizedReference);
	if (!Number.isInteger(legacyIndex) || legacyIndex < 0) {
		return { legacyIndex: null, plan: undefined };
	}

	const legacyPlan = plans.find((plan) => plan.sort_order === legacyIndex);
	return {
		legacyIndex,
		plan: legacyPlan,
	};
}
