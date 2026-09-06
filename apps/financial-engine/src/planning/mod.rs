use std::collections::HashSet;

use crate::date::Date;
use crate::forecast::{
    Frequency, MAX_PROJECTION_EVENTS, ProjectionInput, ProjectionKind, ProjectionSource,
    build_projection,
};
use crate::money::{Currency, checked_add, checked_sub, mul_div_round_half_even_nonnegative};
use crate::{EngineError, EngineResult};

const MAX_PLANNING_ITEMS: usize = 300;
const MAX_PLANNING_ALLOCATIONS: usize = 100;
const MAX_IDENTIFIER_BYTES: usize = 128;
const MAX_NAME_BYTES: usize = 200;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PlanningItemKind {
    Income,
    Bill,
    Subscription,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PlanningItem {
    pub id: String,
    pub name: String,
    pub kind: PlanningItemKind,
    pub expected_amount_minor: i64,
    pub next_date: Date,
    pub frequency: Frequency,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PlanningAllocation {
    pub id: String,
    pub name: String,
    pub amount_minor: i64,
    pub frequency: Frequency,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PlanningSummaryInput {
    pub currency: Currency,
    pub as_of_date: Date,
    pub plan_start_date: Date,
    pub plan_end_date: Date,
    pub items: Vec<PlanningItem>,
    pub allocations: Vec<PlanningAllocation>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PlanningAllocationAmount {
    pub id: String,
    pub name: String,
    pub amount_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PlanningSummaryOutput {
    pub currency: Currency,
    pub expected_income_minor: i64,
    pub committed_bills_minor: i64,
    pub committed_subscriptions_minor: i64,
    pub planned_minor: i64,
    pub unallocated_minor: i64,
    pub recurring_bills_monthly_minor: i64,
    pub recurring_subscriptions_monthly_minor: i64,
    pub recurring_monthly_minor: i64,
    pub recurring_share_percent: u32,
    pub next_7_days_minor: i64,
    pub next_30_days_minor: i64,
    pub annual_cost_minor: i64,
    pub allocations: Vec<PlanningAllocationAmount>,
}

pub fn calculate_planning_summary(
    input: &PlanningSummaryInput,
) -> EngineResult<PlanningSummaryOutput> {
    validate_input(input)?;
    let sources = input
        .items
        .iter()
        .map(projection_source)
        .collect::<EngineResult<Vec<_>>>()?;
    let plan = projection(
        input.currency,
        input.plan_start_date,
        input.plan_end_date,
        sources.clone(),
    )?;

    let mut expected_income_minor = 0_i64;
    let mut committed_bills_minor = 0_i64;
    let mut committed_subscriptions_minor = 0_i64;
    for event in plan.events {
        match event.kind {
            ProjectionKind::Salary if event.amount_minor > 0 => {
                expected_income_minor = checked_add(expected_income_minor, event.amount_minor)?;
            }
            ProjectionKind::Bill if event.amount_minor < 0 => {
                committed_bills_minor = checked_add(
                    committed_bills_minor,
                    event
                        .amount_minor
                        .checked_abs()
                        .ok_or(EngineError::ArithmeticOverflow)?,
                )?;
            }
            ProjectionKind::Subscription if event.amount_minor < 0 => {
                committed_subscriptions_minor = checked_add(
                    committed_subscriptions_minor,
                    event
                        .amount_minor
                        .checked_abs()
                        .ok_or(EngineError::ArithmeticOverflow)?,
                )?;
            }
            _ => return Err(EngineError::InvalidInput("planning event is invalid")),
        }
    }

    let allocations = input
        .allocations
        .iter()
        .map(|allocation| {
            Ok(PlanningAllocationAmount {
                id: allocation.id.clone(),
                name: allocation.name.clone(),
                amount_minor: monthly_equivalent(allocation.amount_minor, allocation.frequency)?,
            })
        })
        .collect::<EngineResult<Vec<_>>>()?;
    let planned_minor = allocations.iter().try_fold(0_i64, |total, allocation| {
        checked_add(total, allocation.amount_minor)
    })?;

    let mut annual_bills_minor = 0_i64;
    let mut annual_subscriptions_minor = 0_i64;
    for item in &input.items {
        if item.kind == PlanningItemKind::Income {
            continue;
        }
        let annual = annual_cost(item.expected_amount_minor, item.frequency)?;
        match item.kind {
            PlanningItemKind::Bill => {
                annual_bills_minor = checked_add(annual_bills_minor, annual)?;
            }
            PlanningItemKind::Subscription => {
                annual_subscriptions_minor = checked_add(annual_subscriptions_minor, annual)?;
            }
            PlanningItemKind::Income => {}
        }
    }
    let recurring_bills_monthly_minor =
        mul_div_round_half_even_nonnegative(annual_bills_minor, 1, 12)?;
    let recurring_subscriptions_monthly_minor =
        mul_div_round_half_even_nonnegative(annual_subscriptions_minor, 1, 12)?;
    let recurring_monthly_minor = checked_add(
        recurring_bills_monthly_minor,
        recurring_subscriptions_monthly_minor,
    )?;
    let annual_cost_minor = checked_add(annual_bills_minor, annual_subscriptions_minor)?;
    let recurring_share_percent = if expected_income_minor == 0 {
        0
    } else {
        u32::try_from(mul_div_round_half_even_nonnegative(
            recurring_monthly_minor,
            100,
            expected_income_minor,
        )?)
        .map_err(|_| EngineError::ArithmeticOverflow)?
    };

    let next_7_end = input.as_of_date.add_days(6)?;
    let next_30_end = input.as_of_date.add_days(29)?;
    let upcoming = projection(input.currency, input.as_of_date, next_30_end, sources)?;
    let mut next_7_days_minor = 0_i64;
    let mut next_30_days_minor = 0_i64;
    for event in upcoming.events {
        if !matches!(
            event.kind,
            ProjectionKind::Bill | ProjectionKind::Subscription
        ) || event.amount_minor >= 0
        {
            continue;
        }
        let amount = event
            .amount_minor
            .checked_abs()
            .ok_or(EngineError::ArithmeticOverflow)?;
        next_30_days_minor = checked_add(next_30_days_minor, amount)?;
        if event.date <= next_7_end {
            next_7_days_minor = checked_add(next_7_days_minor, amount)?;
        }
    }

    let unallocated_minor = checked_sub(
        checked_sub(
            checked_sub(expected_income_minor, committed_bills_minor)?,
            committed_subscriptions_minor,
        )?,
        planned_minor,
    )?;

    Ok(PlanningSummaryOutput {
        currency: input.currency,
        expected_income_minor,
        committed_bills_minor,
        committed_subscriptions_minor,
        planned_minor,
        unallocated_minor,
        recurring_bills_monthly_minor,
        recurring_subscriptions_monthly_minor,
        recurring_monthly_minor,
        recurring_share_percent,
        next_7_days_minor,
        next_30_days_minor,
        annual_cost_minor,
        allocations,
    })
}

fn validate_input(input: &PlanningSummaryInput) -> EngineResult<()> {
    input.as_of_date.validate()?;
    input.plan_start_date.validate()?;
    input.plan_end_date.validate()?;
    if input.plan_start_date.day != 1
        || input.plan_end_date != input.plan_start_date.add_months(1)?.previous_day()?
        || input.plan_start_date <= input.as_of_date
    {
        return Err(EngineError::InvalidInput("planning period is invalid"));
    }
    if input.items.len() > MAX_PLANNING_ITEMS {
        return Err(EngineError::LimitExceeded {
            resource: "planning items",
            limit: MAX_PLANNING_ITEMS,
        });
    }
    if input.allocations.len() > MAX_PLANNING_ALLOCATIONS {
        return Err(EngineError::LimitExceeded {
            resource: "planning allocations",
            limit: MAX_PLANNING_ALLOCATIONS,
        });
    }
    for item in &input.items {
        if item.expected_amount_minor <= 0 {
            return Err(EngineError::InvalidInput(
                "planning item amount must be positive",
            ));
        }
    }
    let mut allocation_ids = HashSet::with_capacity(input.allocations.len());
    for allocation in &input.allocations {
        if allocation.id.is_empty()
            || allocation.id.len() > MAX_IDENTIFIER_BYTES
            || allocation.name.is_empty()
            || allocation.name.len() > MAX_NAME_BYTES
            || allocation.amount_minor < 0
            || allocation.frequency == Frequency::Once
            || !allocation_ids.insert(allocation.id.as_str())
        {
            return Err(EngineError::InvalidInput("planning allocation is invalid"));
        }
    }
    Ok(())
}

fn projection_source(item: &PlanningItem) -> EngineResult<ProjectionSource> {
    let (kind, amount_minor) = match item.kind {
        PlanningItemKind::Income => (ProjectionKind::Salary, item.expected_amount_minor),
        PlanningItemKind::Bill => (
            ProjectionKind::Bill,
            item.expected_amount_minor
                .checked_neg()
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
        PlanningItemKind::Subscription => (
            ProjectionKind::Subscription,
            item.expected_amount_minor
                .checked_neg()
                .ok_or(EngineError::ArithmeticOverflow)?,
        ),
    };
    Ok(ProjectionSource {
        source_id: item.id.clone(),
        name: item.name.clone(),
        kind,
        amount_minor,
        first_date: item.next_date,
        frequency: item.frequency,
        end_date: None,
    })
}

fn projection(
    currency: Currency,
    start_date: Date,
    end_date: Date,
    sources: Vec<ProjectionSource>,
) -> EngineResult<crate::forecast::ProjectionOutput> {
    build_projection(&ProjectionInput {
        currency,
        starting_balance_minor: 0,
        start_date,
        end_date,
        safety_floor_minor: 0,
        max_events: u32::try_from(MAX_PROJECTION_EVENTS)
            .map_err(|_| EngineError::ArithmeticOverflow)?,
        sources,
    })
}

fn annual_cost(amount_minor: i64, frequency: Frequency) -> EngineResult<i64> {
    let occurrences = match frequency {
        Frequency::Once => 1,
        Frequency::Weekly => 52,
        Frequency::Biweekly => 26,
        Frequency::Monthly => 12,
        Frequency::Quarterly => 4,
        Frequency::Annually => 1,
    };
    amount_minor
        .checked_mul(occurrences)
        .ok_or(EngineError::ArithmeticOverflow)
}

fn monthly_equivalent(amount_minor: i64, frequency: Frequency) -> EngineResult<i64> {
    mul_div_round_half_even_nonnegative(
        amount_minor,
        match frequency {
            Frequency::Weekly => 52,
            Frequency::Biweekly => 26,
            Frequency::Monthly => 12,
            Frequency::Quarterly => 4,
            Frequency::Annually => 1,
            Frequency::Once => {
                return Err(EngineError::InvalidInput("planning allocation must recur"));
            }
        },
        12,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn calculates_next_month_plan_and_recurring_windows() {
        let output = calculate_planning_summary(&PlanningSummaryInput {
            currency: Currency::Cad,
            as_of_date: Date::new(2000, 8, 24).unwrap(),
            plan_start_date: Date::new(2000, 9, 1).unwrap(),
            plan_end_date: Date::new(2000, 9, 30).unwrap(),
            items: vec![
                item("income", PlanningItemKind::Income, 5_500_00, 1),
                item("rent", PlanningItemKind::Bill, 1_800_00, 1),
                item("car", PlanningItemKind::Bill, 420_00, 27),
                item("synthetic-subscription", PlanningItemKind::Subscription, 1_23, 24),
            ],
            allocations: vec![PlanningAllocation {
                id: "daily".into(),
                name: "Daily".into(),
                amount_minor: 1_200_00,
                frequency: Frequency::Monthly,
            }],
        })
        .unwrap();

        assert_eq!(output.expected_income_minor, 5_500_00);
        assert_eq!(output.committed_bills_minor, 2_220_00);
        assert_eq!(output.committed_subscriptions_minor, 1_23);
        assert_eq!(output.planned_minor, 1_200_00);
        assert_eq!(output.unallocated_minor, 2_078_77);
        assert_eq!(output.recurring_monthly_minor, 2_221_23);
        assert_eq!(output.next_7_days_minor, 421_23);
        assert_eq!(output.next_30_days_minor, 2_221_23);
        assert_eq!(output.annual_cost_minor, 26_654_76);
        assert_eq!(output.recurring_share_percent, 40);
    }

    #[test]
    fn rejects_unbounded_planning_inputs() {
        let error = calculate_planning_summary(&PlanningSummaryInput {
            currency: Currency::Cad,
            as_of_date: Date::new(2000, 8, 24).unwrap(),
            plan_start_date: Date::new(2000, 9, 1).unwrap(),
            plan_end_date: Date::new(2000, 9, 30).unwrap(),
            items: (0..=MAX_PLANNING_ITEMS)
                .map(|index| item(&format!("bill-{index}"), PlanningItemKind::Bill, 100, 1))
                .collect(),
            allocations: Vec::new(),
        })
        .unwrap_err();

        assert_eq!(
            error,
            EngineError::LimitExceeded {
                resource: "planning items",
                limit: MAX_PLANNING_ITEMS,
            }
        );
    }

    fn item(id: &str, kind: PlanningItemKind, amount: i64, day: u8) -> PlanningItem {
        PlanningItem {
            id: id.into(),
            name: id.into(),
            kind,
            expected_amount_minor: amount,
            next_date: Date::new(2000, 8, day).unwrap(),
            frequency: Frequency::Monthly,
        }
    }
}
