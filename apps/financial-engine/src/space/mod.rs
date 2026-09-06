use serde::{Deserialize, Serialize};

use crate::forecast::ProjectionStatus;
use crate::money::{Currency, checked_add, checked_sub, checked_sum};
use crate::{EngineError, EngineResult};

pub const MAX_BREAKDOWN_ITEMS: usize = 512;
const MAX_IDENTIFIER_BYTES: usize = 128;
const MAX_LABEL_BYTES: usize = 200;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ProtectedKind {
    UpcomingBill,
    Subscription,
    ProtectedSpace,
    RequiredGoalContribution,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProtectedItem {
    pub id: String,
    pub label: String,
    pub kind: ProtectedKind,
    pub amount_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct PlannedAllocation {
    pub id: String,
    pub label: String,
    pub amount_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AvailableToSpendInput {
    pub currency: Currency,
    pub liquid_balance_minor: i64,
    pub eligible_expected_income_minor: i64,
    pub protected_items: Vec<ProtectedItem>,
    pub planned_allocations: Vec<PlannedAllocation>,
    pub safety_buffer_minor: i64,
    pub projection_status: ProjectionStatus,
    pub minimum_projected_balance_minor: i64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum AvailableComponentKind {
    LiquidBalance,
    ExpectedIncome,
    UpcomingBill,
    Subscription,
    ProtectedSpace,
    RequiredGoalContribution,
    SafetyBuffer,
    PlannedAllocation,
    PayCycleReserve,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AvailableComponent {
    pub id: String,
    pub label: String,
    pub kind: AvailableComponentKind,
    pub amount_minor: i64,
    pub effect_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AvailableToSpendOutput {
    pub currency: Currency,
    pub available_minor: i64,
    pub total_minor: i64,
    pub protected_minor: i64,
    pub planned_allocations_minor: i64,
    pub status: ProjectionStatus,
    pub breakdown: Vec<AvailableComponent>,
}

pub fn calculate_available_to_spend(
    input: &AvailableToSpendInput,
) -> EngineResult<AvailableToSpendOutput> {
    let item_count = input
        .protected_items
        .len()
        .checked_add(input.planned_allocations.len())
        .ok_or(EngineError::ArithmeticOverflow)?;
    if item_count > MAX_BREAKDOWN_ITEMS {
        return Err(EngineError::LimitExceeded {
            resource: "available-to-spend breakdown items",
            limit: MAX_BREAKDOWN_ITEMS,
        });
    }
    if input.eligible_expected_income_minor < 0 || input.safety_buffer_minor < 0 {
        return Err(EngineError::InvalidInput(
            "expected income and the safety buffer cannot be negative",
        ));
    }
    validate_items(&input.protected_items, &input.planned_allocations)?;

    let scheduled_obligations_minor =
        checked_sum(input.protected_items.iter().filter_map(|item| {
            matches!(
                item.kind,
                ProtectedKind::UpcomingBill | ProtectedKind::Subscription
            )
            .then_some(item.amount_minor)
        }))?;
    let projected_ending_balance_minor = checked_sub(
        checked_add(
            input.liquid_balance_minor,
            input.eligible_expected_income_minor,
        )?,
        scheduled_obligations_minor,
    )?;
    if input.minimum_projected_balance_minor > projected_ending_balance_minor {
        return Err(EngineError::InvalidInput(
            "minimum projected balance exceeds the projected ending balance",
        ));
    }
    let pay_cycle_reserve_minor = checked_sub(
        projected_ending_balance_minor,
        input.minimum_projected_balance_minor,
    )?;
    let protected_minor = checked_add(
        checked_sum(input.protected_items.iter().map(|item| item.amount_minor))?,
        checked_add(input.safety_buffer_minor, pay_cycle_reserve_minor)?,
    )?;
    let planned_allocations_minor = checked_sum(
        input
            .planned_allocations
            .iter()
            .map(|item| item.amount_minor),
    )?;
    let total_minor = checked_add(
        input.liquid_balance_minor,
        input.eligible_expected_income_minor,
    )?;
    let available_minor = checked_sub(
        checked_sub(total_minor, protected_minor)?,
        planned_allocations_minor,
    )?;
    let status = if available_minor < 0 {
        ProjectionStatus::AtRisk
    } else {
        input.projection_status
    };

    let mut breakdown = Vec::with_capacity(item_count + 3);
    breakdown.push(AvailableComponent {
        id: "liquid_balance".to_owned(),
        label: "Liquid balance".to_owned(),
        kind: AvailableComponentKind::LiquidBalance,
        amount_minor: input.liquid_balance_minor,
        effect_minor: input.liquid_balance_minor,
    });
    if input.eligible_expected_income_minor != 0 {
        breakdown.push(AvailableComponent {
            id: "eligible_expected_income".to_owned(),
            label: "Eligible expected income".to_owned(),
            kind: AvailableComponentKind::ExpectedIncome,
            amount_minor: input.eligible_expected_income_minor,
            effect_minor: input.eligible_expected_income_minor,
        });
    }
    for item in &input.protected_items {
        breakdown.push(AvailableComponent {
            id: item.id.clone(),
            label: item.label.clone(),
            kind: match item.kind {
                ProtectedKind::UpcomingBill => AvailableComponentKind::UpcomingBill,
                ProtectedKind::Subscription => AvailableComponentKind::Subscription,
                ProtectedKind::ProtectedSpace => AvailableComponentKind::ProtectedSpace,
                ProtectedKind::RequiredGoalContribution => {
                    AvailableComponentKind::RequiredGoalContribution
                }
            },
            amount_minor: item.amount_minor,
            effect_minor: item
                .amount_minor
                .checked_neg()
                .ok_or(EngineError::ArithmeticOverflow)?,
        });
    }
    if pay_cycle_reserve_minor != 0 {
        breakdown.push(AvailableComponent {
            id: "pay_cycle_reserve".to_owned(),
            label: "Pay-cycle timing reserve".to_owned(),
            kind: AvailableComponentKind::PayCycleReserve,
            amount_minor: pay_cycle_reserve_minor,
            effect_minor: pay_cycle_reserve_minor
                .checked_neg()
                .ok_or(EngineError::ArithmeticOverflow)?,
        });
    }
    if input.safety_buffer_minor != 0 {
        breakdown.push(AvailableComponent {
            id: "safety_buffer".to_owned(),
            label: "Safety buffer".to_owned(),
            kind: AvailableComponentKind::SafetyBuffer,
            amount_minor: input.safety_buffer_minor,
            effect_minor: input
                .safety_buffer_minor
                .checked_neg()
                .ok_or(EngineError::ArithmeticOverflow)?,
        });
    }
    for item in &input.planned_allocations {
        breakdown.push(AvailableComponent {
            id: item.id.clone(),
            label: item.label.clone(),
            kind: AvailableComponentKind::PlannedAllocation,
            amount_minor: item.amount_minor,
            effect_minor: item
                .amount_minor
                .checked_neg()
                .ok_or(EngineError::ArithmeticOverflow)?,
        });
    }

    Ok(AvailableToSpendOutput {
        currency: input.currency,
        available_minor,
        total_minor,
        protected_minor,
        planned_allocations_minor,
        status,
        breakdown,
    })
}

fn validate_items(
    protected_items: &[ProtectedItem],
    planned_allocations: &[PlannedAllocation],
) -> EngineResult<()> {
    for item in protected_items {
        validate_item(&item.id, &item.label, item.amount_minor)?;
    }
    for item in planned_allocations {
        validate_item(&item.id, &item.label, item.amount_minor)?;
    }
    Ok(())
}

fn validate_item(id: &str, label: &str, amount_minor: i64) -> EngineResult<()> {
    if id.is_empty()
        || id.len() > MAX_IDENTIFIER_BYTES
        || label.is_empty()
        || label.len() > MAX_LABEL_BYTES
        || amount_minor < 0
    {
        return Err(EngineError::InvalidInput("breakdown item is invalid"));
    }
    Ok(())
}
