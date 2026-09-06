use std::collections::{BTreeMap, HashSet};

use serde::{Deserialize, Serialize};

use crate::money::{Currency, checked_add, checked_sub};
use crate::{EngineError, EngineResult};

pub const MAX_HOUSEHOLD_MEMBERS: usize = 100;
pub const MAX_HOUSEHOLD_OBLIGATION_ENTRIES: usize = 4_096;
const MAX_IDENTIFIER_BYTES: usize = 128;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct HouseholdObligationEntry {
    pub obligation_id: String,
    pub member_id: String,
    pub paid_minor: i64,
    pub share_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct HouseholdSettlementInput {
    pub currency: Currency,
    pub entries: Vec<HouseholdObligationEntry>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct HouseholdMemberPosition {
    pub member_id: String,
    pub total_paid_minor: i64,
    pub total_share_minor: i64,
    pub net_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct HouseholdSettlementTransfer {
    pub from_member_id: String,
    pub to_member_id: String,
    pub amount_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct HouseholdSettlementOutput {
    pub currency: Currency,
    pub positions: Vec<HouseholdMemberPosition>,
    pub transfers: Vec<HouseholdSettlementTransfer>,
}

pub fn calculate_household_settlement(
    input: &HouseholdSettlementInput,
) -> EngineResult<HouseholdSettlementOutput> {
    if input.entries.len() > MAX_HOUSEHOLD_OBLIGATION_ENTRIES {
        return Err(EngineError::LimitExceeded {
            resource: "household obligation entries",
            limit: MAX_HOUSEHOLD_OBLIGATION_ENTRIES,
        });
    }

    let mut seen_entries = HashSet::with_capacity(input.entries.len());
    let mut obligation_totals: BTreeMap<&str, (i64, i64)> = BTreeMap::new();
    let mut member_totals: BTreeMap<&str, (i64, i64)> = BTreeMap::new();
    for entry in &input.entries {
        validate_identifier(&entry.obligation_id)?;
        validate_identifier(&entry.member_id)?;
        if !seen_entries.insert((entry.obligation_id.as_str(), entry.member_id.as_str())) {
            return Err(EngineError::InvalidInput(
                "household obligation contains a duplicate member",
            ));
        }

        let obligation = obligation_totals
            .entry(entry.obligation_id.as_str())
            .or_insert((0, 0));
        obligation.0 = checked_add(obligation.0, entry.paid_minor)?;
        obligation.1 = checked_add(obligation.1, entry.share_minor)?;

        let member = member_totals
            .entry(entry.member_id.as_str())
            .or_insert((0, 0));
        member.0 = checked_add(member.0, entry.paid_minor)?;
        member.1 = checked_add(member.1, entry.share_minor)?;
    }
    if member_totals.len() > MAX_HOUSEHOLD_MEMBERS {
        return Err(EngineError::LimitExceeded {
            resource: "household members",
            limit: MAX_HOUSEHOLD_MEMBERS,
        });
    }
    if obligation_totals
        .values()
        .any(|(paid, share)| paid != share)
    {
        return Err(EngineError::InvalidInput(
            "household obligation paid and share totals must balance",
        ));
    }

    let mut positions = Vec::with_capacity(member_totals.len());
    let mut debtors = Vec::new();
    let mut creditors = Vec::new();
    for (member_id, (paid, share)) in member_totals {
        let net = checked_sub(paid, share)?;
        positions.push(HouseholdMemberPosition {
            member_id: member_id.to_owned(),
            total_paid_minor: paid,
            total_share_minor: share,
            net_minor: net,
        });
        if net < 0 {
            debtors.push((
                member_id.to_owned(),
                net.checked_neg().ok_or(EngineError::ArithmeticOverflow)?,
            ));
        } else if net > 0 {
            creditors.push((member_id.to_owned(), net));
        }
    }
    sort_open_positions(&mut debtors);
    sort_open_positions(&mut creditors);
    let transfers = build_transfers(&mut debtors, &mut creditors)?;

    Ok(HouseholdSettlementOutput {
        currency: input.currency,
        positions,
        transfers,
    })
}

fn sort_open_positions(positions: &mut [(String, i64)]) {
    positions.sort_by(|left, right| right.1.cmp(&left.1).then_with(|| left.0.cmp(&right.0)));
}

fn build_transfers(
    debtors: &mut [(String, i64)],
    creditors: &mut [(String, i64)],
) -> EngineResult<Vec<HouseholdSettlementTransfer>> {
    let capacity = debtors
        .len()
        .checked_add(creditors.len())
        .and_then(|count| count.checked_sub(1))
        .unwrap_or(0);
    let mut transfers = Vec::with_capacity(capacity);
    let mut debtor_index = 0;
    let mut creditor_index = 0;
    while debtor_index < debtors.len() && creditor_index < creditors.len() {
        let amount = debtors[debtor_index].1.min(creditors[creditor_index].1);
        if amount <= 0 {
            return Err(EngineError::ArithmeticOverflow);
        }
        transfers.push(HouseholdSettlementTransfer {
            from_member_id: debtors[debtor_index].0.clone(),
            to_member_id: creditors[creditor_index].0.clone(),
            amount_minor: amount,
        });
        debtors[debtor_index].1 = checked_sub(debtors[debtor_index].1, amount)?;
        creditors[creditor_index].1 = checked_sub(creditors[creditor_index].1, amount)?;
        if debtors[debtor_index].1 == 0 {
            debtor_index += 1;
        }
        if creditors[creditor_index].1 == 0 {
            creditor_index += 1;
        }
    }
    if debtors.iter().any(|position| position.1 != 0)
        || creditors.iter().any(|position| position.1 != 0)
    {
        return Err(EngineError::InvalidInput(
            "household member net positions do not balance",
        ));
    }
    Ok(transfers)
}

fn validate_identifier(value: &str) -> EngineResult<()> {
    if value.is_empty() || value.len() > MAX_IDENTIFIER_BYTES {
        return Err(EngineError::InvalidInput(
            "household settlement identifier is outside the supported range",
        ));
    }
    Ok(())
}
