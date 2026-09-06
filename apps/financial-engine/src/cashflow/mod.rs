use serde::{Deserialize, Serialize};

use crate::money::{Currency, checked_sub, checked_sum};
use crate::{EngineError, EngineResult};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CashFlowComponents {
    pub income_minor: i64,
    pub fixed_outflow_minor: i64,
    pub variable_outflow_minor: i64,
    pub subscriptions_minor: i64,
    pub savings_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CashFlowInput {
    pub currency: Currency,
    pub actual: CashFlowComponents,
    pub projected: CashFlowComponents,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CashFlowSummary {
    pub income_minor: i64,
    pub fixed_outflow_minor: i64,
    pub variable_outflow_minor: i64,
    pub subscriptions_minor: i64,
    pub savings_minor: i64,
    pub total_outflow_minor: i64,
    pub net_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CashFlowOutput {
    pub currency: Currency,
    pub actual: CashFlowSummary,
    pub projected: CashFlowSummary,
}

pub fn calculate_cash_flow(input: &CashFlowInput) -> EngineResult<CashFlowOutput> {
    Ok(CashFlowOutput {
        currency: input.currency,
        actual: summarize(&input.actual)?,
        projected: summarize(&input.projected)?,
    })
}

fn summarize(components: &CashFlowComponents) -> EngineResult<CashFlowSummary> {
    if components.income_minor < 0
        || components.fixed_outflow_minor < 0
        || components.variable_outflow_minor < 0
        || components.subscriptions_minor < 0
        || components.savings_minor < 0
    {
        return Err(EngineError::InvalidInput(
            "cash-flow components cannot be negative",
        ));
    }
    let total_outflow_minor = checked_sum([
        components.fixed_outflow_minor,
        components.variable_outflow_minor,
        components.subscriptions_minor,
        components.savings_minor,
    ])?;
    Ok(CashFlowSummary {
        income_minor: components.income_minor,
        fixed_outflow_minor: components.fixed_outflow_minor,
        variable_outflow_minor: components.variable_outflow_minor,
        subscriptions_minor: components.subscriptions_minor,
        savings_minor: components.savings_minor,
        total_outflow_minor,
        net_minor: checked_sub(components.income_minor, total_outflow_minor)?,
    })
}
