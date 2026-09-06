use std::collections::HashSet;

use serde::{Deserialize, Serialize};

use crate::money::Currency;
use crate::{EngineError, EngineResult};

pub const MAX_RULES: usize = 256;
pub const MAX_CONDITIONS_PER_RULE: usize = 20;
pub const MAX_ACTIONS_PER_RULE: usize = 20;
pub const MAX_EVALUATED_ACTIONS: usize = MAX_RULES * MAX_ACTIONS_PER_RULE;
const MAX_IDENTIFIER_BYTES: usize = 128;
const MAX_VALUE_BYTES: usize = 200;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ConditionField {
    Merchant,
    Amount,
    Category,
    Account,
    TransactionType,
    IncomeSource,
    Space,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ConditionOperator {
    Contains,
    IsExactly,
    StartsWith,
    GreaterThan,
    LessThan,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "kind", content = "value", rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ConditionValue {
    Text(String),
    AmountMinor(i64),
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Condition {
    pub field: ConditionField,
    pub operator: ConditionOperator,
    pub value: ConditionValue,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum ActionKind {
    SetCategory,
    AllocateSpace,
    AddTag,
    SetSplit,
    MarkReviewed,
    CreateNotification,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Action {
    pub kind: ActionKind,
    pub value: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Rule {
    pub rule_id: String,
    pub priority: u32,
    pub enabled: bool,
    pub conditions: Vec<Condition>,
    pub actions: Vec<Action>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RuleTransaction {
    pub transaction_id: String,
    pub currency: Currency,
    pub merchant: Option<String>,
    pub amount_minor: i64,
    pub category: Option<String>,
    pub account: Option<String>,
    pub transaction_type: Option<String>,
    pub income_source: Option<String>,
    pub space: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct EvaluateRulesInput {
    pub transaction: RuleTransaction,
    pub rules: Vec<Rule>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct EvaluatedAction {
    pub rule_id: String,
    pub priority: u32,
    pub action_index: u32,
    pub action: Action,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct EvaluateRulesOutput {
    pub matched_rule_ids: Vec<String>,
    pub actions: Vec<EvaluatedAction>,
}

pub fn evaluate_rules(input: &EvaluateRulesInput) -> EngineResult<EvaluateRulesOutput> {
    validate_input(input)?;
    let mut rules: Vec<&Rule> = input.rules.iter().filter(|rule| rule.enabled).collect();
    rules.sort_by(|left, right| {
        left.priority
            .cmp(&right.priority)
            .then_with(|| left.rule_id.cmp(&right.rule_id))
    });

    let mut matched_rule_ids = Vec::new();
    let mut actions = Vec::new();
    for rule in rules {
        let mut matched = true;
        for condition in &rule.conditions {
            if !condition_matches(condition, &input.transaction)? {
                matched = false;
                break;
            }
        }
        if !matched {
            continue;
        }
        matched_rule_ids.push(rule.rule_id.clone());
        for (index, action) in rule.actions.iter().enumerate() {
            if actions.len() >= MAX_EVALUATED_ACTIONS {
                return Err(EngineError::LimitExceeded {
                    resource: "evaluated rule actions",
                    limit: MAX_EVALUATED_ACTIONS,
                });
            }
            actions.push(EvaluatedAction {
                rule_id: rule.rule_id.clone(),
                priority: rule.priority,
                action_index: u32::try_from(index).map_err(|_| EngineError::ArithmeticOverflow)?,
                action: action.clone(),
            });
        }
    }
    Ok(EvaluateRulesOutput {
        matched_rule_ids,
        actions,
    })
}

fn validate_input(input: &EvaluateRulesInput) -> EngineResult<()> {
    if input.rules.len() > MAX_RULES {
        return Err(EngineError::LimitExceeded {
            resource: "rules",
            limit: MAX_RULES,
        });
    }
    validate_identifier(&input.transaction.transaction_id)?;
    for value in [
        input.transaction.merchant.as_deref(),
        input.transaction.category.as_deref(),
        input.transaction.account.as_deref(),
        input.transaction.transaction_type.as_deref(),
        input.transaction.income_source.as_deref(),
        input.transaction.space.as_deref(),
    ]
    .into_iter()
    .flatten()
    {
        validate_text(value)?;
    }

    let mut rule_ids = HashSet::with_capacity(input.rules.len());
    for rule in &input.rules {
        validate_identifier(&rule.rule_id)?;
        if rule.priority == 0
            || rule.conditions.is_empty()
            || rule.conditions.len() > MAX_CONDITIONS_PER_RULE
            || rule.actions.is_empty()
            || rule.actions.len() > MAX_ACTIONS_PER_RULE
        {
            return Err(EngineError::InvalidInput(
                "rule is outside the supported range",
            ));
        }
        if !rule_ids.insert(rule.rule_id.as_str()) {
            return Err(EngineError::InvalidInput("rule IDs must be unique"));
        }
        for condition in &rule.conditions {
            validate_condition(condition)?;
        }
        for action in &rule.actions {
            if let Some(value) = &action.value
                && value.len() > MAX_VALUE_BYTES
            {
                return Err(EngineError::InvalidInput("rule action value is too long"));
            }
        }
    }
    Ok(())
}

fn validate_condition(condition: &Condition) -> EngineResult<()> {
    match (&condition.field, &condition.operator, &condition.value) {
        (
            ConditionField::Amount,
            ConditionOperator::IsExactly
            | ConditionOperator::GreaterThan
            | ConditionOperator::LessThan,
            ConditionValue::AmountMinor(_),
        ) => Ok(()),
        (
            ConditionField::Merchant
            | ConditionField::Category
            | ConditionField::Account
            | ConditionField::TransactionType
            | ConditionField::IncomeSource
            | ConditionField::Space,
            ConditionOperator::Contains
            | ConditionOperator::IsExactly
            | ConditionOperator::StartsWith,
            ConditionValue::Text(value),
        ) => validate_text(value),
        _ => Err(EngineError::InvalidInput(
            "rule condition field, operator, and value are incompatible",
        )),
    }
}

fn condition_matches(condition: &Condition, transaction: &RuleTransaction) -> EngineResult<bool> {
    match (&condition.field, &condition.value) {
        (ConditionField::Amount, ConditionValue::AmountMinor(expected)) => {
            Ok(match condition.operator {
                ConditionOperator::IsExactly => transaction.amount_minor == *expected,
                ConditionOperator::GreaterThan => transaction.amount_minor > *expected,
                ConditionOperator::LessThan => transaction.amount_minor < *expected,
                _ => false,
            })
        }
        (field, ConditionValue::Text(expected)) => {
            let actual = match field {
                ConditionField::Merchant => transaction.merchant.as_deref(),
                ConditionField::Category => transaction.category.as_deref(),
                ConditionField::Account => transaction.account.as_deref(),
                ConditionField::TransactionType => transaction.transaction_type.as_deref(),
                ConditionField::IncomeSource => transaction.income_source.as_deref(),
                ConditionField::Space => transaction.space.as_deref(),
                ConditionField::Amount => None,
            };
            Ok(actual.is_some_and(|value| text_matches(value, expected, condition.operator)))
        }
        _ => Err(EngineError::InvalidInput(
            "rule condition value is incompatible",
        )),
    }
}

fn text_matches(actual: &str, expected: &str, operator: ConditionOperator) -> bool {
    let actual = actual.to_lowercase();
    let expected = expected.to_lowercase();
    match operator {
        ConditionOperator::Contains => actual.contains(&expected),
        ConditionOperator::IsExactly => actual == expected,
        ConditionOperator::StartsWith => actual.starts_with(&expected),
        ConditionOperator::GreaterThan | ConditionOperator::LessThan => false,
    }
}

fn validate_identifier(value: &str) -> EngineResult<()> {
    if value.is_empty() || value.len() > MAX_IDENTIFIER_BYTES {
        return Err(EngineError::InvalidInput(
            "identifier is outside the supported range",
        ));
    }
    Ok(())
}

fn validate_text(value: &str) -> EngineResult<()> {
    if value.is_empty() || value.len() > MAX_VALUE_BYTES {
        return Err(EngineError::InvalidInput(
            "text value is outside the supported range",
        ));
    }
    Ok(())
}
