use serde::{Deserialize, Serialize};

use crate::date::Date;
use crate::money::{Currency, checked_add, checked_sub, mul_div_round_half_even_nonnegative};
use crate::{EngineError, EngineResult};

pub const MAX_LOAN_MONTHS: usize = 600;
pub const MAX_INTEREST_RATE_BASIS_POINTS: u32 = 100_000;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct LoanScenarioInput {
    pub currency: Currency,
    pub principal_remaining_minor: i64,
    pub annual_interest_rate_basis_points: u32,
    pub monthly_payment_minor: i64,
    pub extra_monthly_payment_minor: i64,
    pub next_payment_date: Date,
    pub include_schedule: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct LoanPayment {
    pub payment_number: u32,
    pub payment_date: Date,
    pub payment_minor: i64,
    pub principal_minor: i64,
    pub interest_minor: i64,
    pub remaining_principal_minor: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AmortizationSummary {
    pub monthly_payment_minor: i64,
    pub payoff_months: u32,
    pub payoff_date: Option<Date>,
    pub total_interest_minor: i64,
    pub total_paid_minor: i64,
    pub schedule: Vec<LoanPayment>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct LoanScenarioOutput {
    pub currency: Currency,
    pub base: AmortizationSummary,
    pub scenario: AmortizationSummary,
    pub months_saved: u32,
    pub interest_saved_minor: i64,
}

pub fn calculate_loan_scenario(input: &LoanScenarioInput) -> EngineResult<LoanScenarioOutput> {
    input.next_payment_date.validate()?;
    if input.principal_remaining_minor < 0
        || input.monthly_payment_minor <= 0
        || input.extra_monthly_payment_minor < 0
        || input.annual_interest_rate_basis_points > MAX_INTEREST_RATE_BASIS_POINTS
    {
        return Err(EngineError::InvalidInput(
            "loan inputs are outside the supported range",
        ));
    }
    let scenario_payment = checked_add(
        input.monthly_payment_minor,
        input.extra_monthly_payment_minor,
    )?;
    let base = amortize(
        input.principal_remaining_minor,
        input.annual_interest_rate_basis_points,
        input.monthly_payment_minor,
        input.next_payment_date,
        input.include_schedule,
    )?;
    let scenario = amortize(
        input.principal_remaining_minor,
        input.annual_interest_rate_basis_points,
        scenario_payment,
        input.next_payment_date,
        input.include_schedule,
    )?;
    Ok(LoanScenarioOutput {
        currency: input.currency,
        months_saved: base.payoff_months.saturating_sub(scenario.payoff_months),
        interest_saved_minor: checked_sub(
            base.total_interest_minor,
            scenario.total_interest_minor,
        )?,
        base,
        scenario,
    })
}

fn amortize(
    principal_minor: i64,
    annual_interest_rate_basis_points: u32,
    monthly_payment_minor: i64,
    next_payment_date: Date,
    include_schedule: bool,
) -> EngineResult<AmortizationSummary> {
    if principal_minor == 0 {
        return Ok(AmortizationSummary {
            monthly_payment_minor,
            payoff_months: 0,
            payoff_date: None,
            total_interest_minor: 0,
            total_paid_minor: 0,
            schedule: Vec::new(),
        });
    }

    let mut balance = principal_minor;
    let mut total_interest = 0_i64;
    let mut total_paid = 0_i64;
    let mut payoff_date = None;
    let mut schedule = if include_schedule {
        Vec::with_capacity(MAX_LOAN_MONTHS.min(120))
    } else {
        Vec::new()
    };
    let mut payoff_months = 0_u32;

    for month_index in 0..MAX_LOAN_MONTHS {
        let interest = mul_div_round_half_even_nonnegative(
            balance,
            i64::from(annual_interest_rate_basis_points),
            120_000,
        )?;
        if monthly_payment_minor <= interest {
            return Err(EngineError::PaymentDoesNotAmortize);
        }
        let balance_with_interest = checked_add(balance, interest)?;
        let payment = monthly_payment_minor.min(balance_with_interest);
        let principal = checked_sub(payment, interest)?;
        balance = checked_sub(balance, principal)?;
        total_interest = checked_add(total_interest, interest)?;
        total_paid = checked_add(total_paid, payment)?;
        let payment_date = next_payment_date
            .add_months(u32::try_from(month_index).map_err(|_| EngineError::ArithmeticOverflow)?)?;
        payoff_months =
            u32::try_from(month_index + 1).map_err(|_| EngineError::ArithmeticOverflow)?;
        if include_schedule {
            schedule.push(LoanPayment {
                payment_number: payoff_months,
                payment_date,
                payment_minor: payment,
                principal_minor: principal,
                interest_minor: interest,
                remaining_principal_minor: balance,
            });
        }
        if balance == 0 {
            payoff_date = Some(payment_date);
            break;
        }
    }
    if balance != 0 {
        return Err(EngineError::PaymentDoesNotAmortize);
    }
    Ok(AmortizationSummary {
        monthly_payment_minor,
        payoff_months,
        payoff_date,
        total_interest_minor: total_interest,
        total_paid_minor: total_paid,
        schedule,
    })
}
