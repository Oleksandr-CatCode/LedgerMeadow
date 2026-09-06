use crate::forecast::Frequency;
use crate::money::{Currency, checked_add};
use crate::{EngineError, EngineResult};

const MAX_TRANSACTION_AMOUNTS: usize = 100;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SubscriptionSummaryInput {
    pub currency: Currency,
    pub expected_amount_minor: i64,
    pub frequency: Frequency,
    pub transaction_amounts_minor: Vec<i64>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SubscriptionSummaryOutput {
    pub currency: Currency,
    pub annual_cost_minor: i64,
    pub paid_this_year_minor: i64,
    pub payment_amounts_minor: Vec<i64>,
}

pub fn calculate_subscription_summary(
    input: &SubscriptionSummaryInput,
) -> EngineResult<SubscriptionSummaryOutput> {
    if input.expected_amount_minor < 0 {
        return Err(EngineError::InvalidInput(
            "expected subscription amount must be nonnegative",
        ));
    }
    if input.transaction_amounts_minor.len() > MAX_TRANSACTION_AMOUNTS {
        return Err(EngineError::LimitExceeded {
            resource: "subscription transaction amounts",
            limit: MAX_TRANSACTION_AMOUNTS,
        });
    }

    let occurrences = match input.frequency {
        Frequency::Weekly => 52_i64,
        Frequency::Biweekly => 26,
        Frequency::Monthly => 12,
        Frequency::Quarterly => 4,
        Frequency::Annually => 1,
        Frequency::Once => {
            return Err(EngineError::InvalidInput(
                "subscription frequency must recur",
            ));
        }
    };
    let annual_cost_minor = input
        .expected_amount_minor
        .checked_mul(occurrences)
        .ok_or(EngineError::ArithmeticOverflow)?;
    let payment_amounts_minor = input.transaction_amounts_minor.iter().map(|amount| {
        if *amount >= 0 {
            return Err(EngineError::InvalidInput(
                "subscription transaction amount must be an outflow",
            ));
        }
        amount.checked_abs().ok_or(EngineError::ArithmeticOverflow)
    });
    let payment_amounts_minor = payment_amounts_minor.collect::<EngineResult<Vec<_>>>()?;
    let paid_this_year_minor = payment_amounts_minor
        .iter()
        .copied()
        .try_fold(0_i64, checked_add)?;

    Ok(SubscriptionSummaryOutput {
        currency: input.currency,
        annual_cost_minor,
        paid_this_year_minor,
        payment_amounts_minor,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn calculates_monthly_subscription_summary() {
        let output = calculate_subscription_summary(&SubscriptionSummaryInput {
            currency: Currency::Cad,
            expected_amount_minor: 3_000,
            frequency: Frequency::Monthly,
            transaction_amounts_minor: vec![-3_000; 8],
        })
        .expect("valid subscription summary");

        assert_eq!(output.annual_cost_minor, 36_000);
        assert_eq!(output.paid_this_year_minor, 24_000);
        assert_eq!(output.payment_amounts_minor, vec![3_000; 8]);
    }

    #[test]
    fn rejects_annual_cost_overflow() {
        let error = calculate_subscription_summary(&SubscriptionSummaryInput {
            currency: Currency::Cad,
            expected_amount_minor: i64::MAX,
            frequency: Frequency::Weekly,
            transaction_amounts_minor: Vec::new(),
        })
        .expect_err("overflow must be rejected");

        assert_eq!(error, EngineError::ArithmeticOverflow);
    }

    #[test]
    fn rejects_unbounded_transaction_amounts() {
        let error = calculate_subscription_summary(&SubscriptionSummaryInput {
            currency: Currency::Cad,
            expected_amount_minor: 3_000,
            frequency: Frequency::Monthly,
            transaction_amounts_minor: vec![-3_000; MAX_TRANSACTION_AMOUNTS + 1],
        })
        .expect_err("unbounded input must be rejected");

        assert_eq!(
            error,
            EngineError::LimitExceeded {
                resource: "subscription transaction amounts",
                limit: MAX_TRANSACTION_AMOUNTS,
            }
        );
    }
}
