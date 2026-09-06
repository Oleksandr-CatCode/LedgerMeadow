use ledgermeadow_financial_engine::budget::{BudgetStatus, BudgetStatusInput, calculate_budget_status};
use ledgermeadow_financial_engine::cashflow::{CashFlowComponents, CashFlowInput, calculate_cash_flow};
use ledgermeadow_financial_engine::date::Date;
use ledgermeadow_financial_engine::debt::{LoanScenarioInput, calculate_loan_scenario};
use ledgermeadow_financial_engine::forecast::{
    Frequency, ProjectionInput, ProjectionKind, ProjectionSource, ProjectionStatus,
    build_projection, pay_cycle_horizon,
};
use ledgermeadow_financial_engine::goals::{GoalInput, GoalStatus, calculate_goal};
use ledgermeadow_financial_engine::household::{
    HouseholdObligationEntry, HouseholdSettlementInput, MAX_HOUSEHOLD_MEMBERS,
    calculate_household_settlement,
};
use ledgermeadow_financial_engine::insights::{
    AnalyticsBreakdownInput, AnalyticsBucket, NetWorthInput, Valuation,
    calculate_analytics_breakdown, calculate_net_worth,
};
use ledgermeadow_financial_engine::money::{Currency, checked_add, mul_div_round_half_even_nonnegative};
use ledgermeadow_financial_engine::rule::{
    Action, ActionKind, Condition, ConditionField, ConditionOperator, ConditionValue,
    EvaluateRulesInput, MAX_RULES, Rule, RuleTransaction, evaluate_rules,
};
use ledgermeadow_financial_engine::space::{
    AvailableToSpendInput, PlannedAllocation, ProtectedItem, ProtectedKind,
    calculate_available_to_spend,
};
use ledgermeadow_financial_engine::{EngineError, EngineResult};

fn date(year: i32, month: u8, day: u8) -> Date {
    Date { year, month, day }
}

#[test]
fn money_arithmetic_is_checked_and_rounds_half_even() {
    assert_eq!(mul_div_round_half_even_nonnegative(1, 5, 2), Ok(2));
    assert_eq!(mul_div_round_half_even_nonnegative(3, 5, 2), Ok(8));
    assert_eq!(
        checked_add(i64::MAX, 1),
        Err(EngineError::ArithmeticOverflow)
    );
}

#[test]
fn rules_are_bounded_and_actions_follow_priority_order() -> EngineResult<()> {
    let matching_condition = Condition {
        field: ConditionField::Merchant,
        operator: ConditionOperator::Contains,
        value: ConditionValue::Text("costco".to_owned()),
    };
    let input = EvaluateRulesInput {
        transaction: RuleTransaction {
            transaction_id: "transaction-1".to_owned(),
            currency: Currency::Cad,
            merchant: Some("Costco Wholesale".to_owned()),
            amount_minor: -14_281,
            category: None,
            account: None,
            transaction_type: None,
            income_source: None,
            space: None,
        },
        rules: vec![
            Rule {
                rule_id: "second".to_owned(),
                priority: 2,
                enabled: true,
                conditions: vec![matching_condition.clone()],
                actions: vec![Action {
                    kind: ActionKind::AllocateSpace,
                    value: Some("household".to_owned()),
                }],
            },
            Rule {
                rule_id: "first".to_owned(),
                priority: 1,
                enabled: true,
                conditions: vec![matching_condition],
                actions: vec![Action {
                    kind: ActionKind::SetCategory,
                    value: Some("groceries".to_owned()),
                }],
            },
        ],
    };
    let output = evaluate_rules(&input)?;
    assert_eq!(output.matched_rule_ids, ["first", "second"]);
    assert_eq!(output.actions[0].rule_id, "first");

    let mut oversized = input;
    let template = oversized.rules[0].clone();
    oversized.rules = vec![template; MAX_RULES + 1];
    assert!(matches!(
        evaluate_rules(&oversized),
        Err(EngineError::LimitExceeded {
            resource: "rules",
            limit: MAX_RULES
        })
    ));
    Ok(())
}

#[test]
fn dashboard_calculations_match_minor_unit_breakdowns() -> EngineResult<()> {
    let budget = calculate_budget_status(&BudgetStatusInput {
        currency: Currency::Cad,
        limit_minor: 65_000,
        spent_minor: 42_100,
        warning_threshold_percent: 75,
        critical_threshold_percent: 90,
        elapsed_units: 20,
        period_units: 31,
    })?;
    assert_eq!(budget.status, BudgetStatus::Fast);
    assert_eq!(budget.projected_spend_minor, 65_255);

    let available = calculate_available_to_spend(&AvailableToSpendInput {
        currency: Currency::Cad,
        liquid_balance_minor: 684_230,
        eligible_expected_income_minor: 0,
        protected_items: vec![ProtectedItem {
            id: "protected".to_owned(),
            label: "Protected obligations".to_owned(),
            kind: ProtectedKind::UpcomingBill,
            amount_minor: 397_750,
        }],
        planned_allocations: vec![PlannedAllocation {
            id: "planned".to_owned(),
            label: "Planned allocations".to_owned(),
            amount_minor: 70_000,
        }],
        safety_buffer_minor: 0,
        projection_status: ProjectionStatus::OnTrack,
        minimum_projected_balance_minor: 286_480,
    })?;
    assert_eq!(available.available_minor, 216_480);
    assert_eq!(available.protected_minor, 397_750);
    assert_eq!(available.status, ProjectionStatus::OnTrack);

    let unavailable = calculate_available_to_spend(&AvailableToSpendInput {
        currency: Currency::Cad,
        liquid_balance_minor: 100_000,
        eligible_expected_income_minor: 0,
        protected_items: vec![ProtectedItem {
            id: "shortfall".to_owned(),
            label: "Scheduled payments".to_owned(),
            kind: ProtectedKind::UpcomingBill,
            amount_minor: 120_000,
        }],
        planned_allocations: vec![],
        safety_buffer_minor: 0,
        projection_status: ProjectionStatus::OnTrack,
        minimum_projected_balance_minor: -20_000,
    })?;
    assert_eq!(unavailable.available_minor, -20_000);
    assert_eq!(unavailable.status, ProjectionStatus::AtRisk);

    let actual = CashFlowComponents {
        income_minor: 580_000,
        fixed_outflow_minor: 241_000,
        variable_outflow_minor: 148_200,
        subscriptions_minor: 14_200,
        savings_minor: 50_000,
    };
    let cash_flow = calculate_cash_flow(&CashFlowInput {
        currency: Currency::Cad,
        actual: actual.clone(),
        projected: actual,
    })?;
    assert_eq!(cash_flow.actual.total_outflow_minor, 453_400);
    assert_eq!(cash_flow.actual.net_minor, 126_600);
    Ok(())
}

#[test]
fn available_to_spend_reserves_cash_flow_timing_shortfall() -> EngineResult<()> {
    let available = calculate_available_to_spend(&AvailableToSpendInput {
        currency: Currency::Cad,
        liquid_balance_minor: 100_000,
        eligible_expected_income_minor: 200_000,
        protected_items: vec![ProtectedItem {
            id: "bill".to_owned(),
            label: "Bill".to_owned(),
            kind: ProtectedKind::UpcomingBill,
            amount_minor: 120_000,
        }],
        planned_allocations: vec![],
        safety_buffer_minor: 0,
        projection_status: ProjectionStatus::OnTrack,
        minimum_projected_balance_minor: 80_000,
    })?;

    assert_eq!(available.available_minor, 80_000);
    assert_eq!(available.protected_minor, 220_000);
    assert_eq!(available.breakdown.last().unwrap().amount_minor, 100_000);
    Ok(())
}

#[test]
fn projection_expands_calendar_recurrence_and_tracks_minimum_balance() -> EngineResult<()> {
    let input = ProjectionInput {
        currency: Currency::Cad,
        starting_balance_minor: 200_000,
        start_date: date(2027, 2, 1),
        end_date: date(2027, 3, 31),
        safety_floor_minor: 150_000,
        max_events: 8,
        sources: vec![
            ProjectionSource {
                source_id: "rent".to_owned(),
                name: "Rent".to_owned(),
                kind: ProjectionKind::Bill,
                amount_minor: -100_000,
                first_date: date(2027, 1, 31),
                frequency: Frequency::Monthly,
                end_date: None,
            },
            ProjectionSource {
                source_id: "salary".to_owned(),
                name: "Salary".to_owned(),
                kind: ProjectionKind::Salary,
                amount_minor: 120_000,
                first_date: date(2027, 2, 28),
                frequency: Frequency::Monthly,
                end_date: None,
            },
        ],
    };
    let output = build_projection(&input)?;
    assert_eq!(output.events.len(), 4);
    assert_eq!(output.events[0].date, date(2027, 2, 28));
    assert_eq!(output.events[0].source_id, "rent");
    assert_eq!(output.events[2].date, date(2027, 3, 28));
    assert_eq!(output.events[3].date, date(2027, 3, 31));
    assert_eq!(output.minimum_balance_minor, 100_000);
    assert_eq!(output.status, ProjectionStatus::Watch);
    let mut limited = input;
    limited.max_events = 1;
    assert!(matches!(
        build_projection(&limited),
        Err(EngineError::LimitExceeded {
            resource: "expanded projection events",
            limit: 1,
        })
    ));
    Ok(())
}

#[test]
fn pay_cycle_horizon_stops_before_the_next_paycheck() -> EngineResult<()> {
    let salary = ProjectionSource {
        source_id: "salary".to_owned(),
        name: "Salary".to_owned(),
        kind: ProjectionKind::Salary,
        amount_minor: 120_000,
        first_date: date(2026, 9, 4),
        frequency: Frequency::Biweekly,
        end_date: None,
    };

    assert_eq!(
        pay_cycle_horizon(date(2026, 8, 24), std::slice::from_ref(&salary))?,
        date(2026, 9, 3)
    );
    assert_eq!(
        pay_cycle_horizon(date(2026, 8, 24), &[])?,
        date(2026, 9, 24)
    );
    Ok(())
}

#[test]
fn paycheck_does_not_cover_an_earlier_obligation() -> EngineResult<()> {
    let sources = vec![
        ProjectionSource {
            source_id: "bill".to_owned(),
            name: "Bill".to_owned(),
            kind: ProjectionKind::Bill,
            amount_minor: -120_000,
            first_date: date(2026, 9, 2),
            frequency: Frequency::Monthly,
            end_date: None,
        },
        ProjectionSource {
            source_id: "salary".to_owned(),
            name: "Salary".to_owned(),
            kind: ProjectionKind::Salary,
            amount_minor: 100_000,
            first_date: date(2026, 9, 4),
            frequency: Frequency::Biweekly,
            end_date: None,
        },
    ];
    let output = build_projection(&ProjectionInput {
        currency: Currency::Cad,
        starting_balance_minor: 110_000,
        start_date: date(2026, 8, 24),
        end_date: pay_cycle_horizon(date(2026, 8, 24), &sources)?,
        safety_floor_minor: 0,
        max_events: 16,
        sources,
    })?;

    assert_eq!(output.ending_balance_minor, -10_000);
    assert_eq!(output.minimum_balance_minor, -10_000);
    assert_eq!(output.status, ProjectionStatus::AtRisk);
    assert_eq!(output.events.len(), 1);
    Ok(())
}

#[test]
fn goal_and_loan_scenarios_are_integer_only_and_bounded() -> EngineResult<()> {
    let goal = calculate_goal(&GoalInput {
        currency: Currency::Cad,
        target_minor: 600_000,
        current_minor: 184_000,
        current_monthly_contribution_minor: 40_000,
        current_date: date(2026, 8, 20),
        target_date: Some(date(2027, 6, 30)),
    })?;
    assert_eq!(goal.status, GoalStatus::OnTrack);
    assert_eq!(goal.contribution_months_remaining, Some(11));
    assert_eq!(goal.required_monthly_contribution_minor, Some(37_819));

    let loan = calculate_loan_scenario(&LoanScenarioInput {
        currency: Currency::Cad,
        principal_remaining_minor: 1_842_000,
        annual_interest_rate_basis_points: 620,
        monthly_payment_minor: 42_000,
        extra_monthly_payment_minor: 10_000,
        next_payment_date: date(2026, 9, 1),
        include_schedule: true,
    })?;
    assert_eq!(loan.base.schedule[0].interest_minor, 9_517);
    assert_eq!(loan.base.schedule[0].principal_minor, 32_483);
    assert!(loan.scenario.payoff_months < loan.base.payoff_months);
    assert!(loan.interest_saved_minor > 0);
    assert_eq!(
        calculate_loan_scenario(&LoanScenarioInput {
            currency: Currency::Cad,
            principal_remaining_minor: 1_842_000,
            annual_interest_rate_basis_points: 620,
            monthly_payment_minor: 100,
            extra_monthly_payment_minor: 0,
            next_payment_date: date(2026, 9, 1),
            include_schedule: false,
        }),
        Err(EngineError::PaymentDoesNotAmortize)
    );
    Ok(())
}

#[test]
fn analytics_shares_are_exact_and_net_worth_is_checked() -> EngineResult<()> {
    let breakdown = calculate_analytics_breakdown(&AnalyticsBreakdownInput {
        currency: Currency::Cad,
        buckets: vec![
            AnalyticsBucket {
                id: "housing".to_owned(),
                label: "Housing".to_owned(),
                amount_minor: 180_000,
                item_count: 1,
            },
            AnalyticsBucket {
                id: "groceries".to_owned(),
                label: "Groceries".to_owned(),
                amount_minor: 48_600,
                item_count: 4,
            },
            AnalyticsBucket {
                id: "other".to_owned(),
                label: "Other".to_owned(),
                amount_minor: 99_800,
                item_count: 9,
            },
        ],
    })?;
    assert_eq!(
        breakdown
            .buckets
            .iter()
            .map(|bucket| bucket.share_basis_points)
            .sum::<u32>(),
        10_000
    );

    let net_worth = calculate_net_worth(&NetWorthInput {
        currency: Currency::Cad,
        assets: vec![Valuation {
            id: "cash".to_owned(),
            label: "Cash".to_owned(),
            amount_minor: 5_210_000,
        }],
        liabilities: vec![Valuation {
            id: "debt".to_owned(),
            label: "Debt".to_owned(),
            amount_minor: 926_000,
        }],
    })?;
    assert_eq!(net_worth.net_worth_minor, 4_284_000);
    Ok(())
}

#[test]
fn household_settlement_nets_signed_obligations_with_bounded_members() -> EngineResult<()> {
    let output = calculate_household_settlement(&HouseholdSettlementInput {
        currency: Currency::Cad,
        entries: vec![
            HouseholdObligationEntry {
                obligation_id: "expense".to_owned(),
                member_id: "synthetic-member-a".to_owned(),
                paid_minor: 10_000,
                share_minor: 5_000,
            },
            HouseholdObligationEntry {
                obligation_id: "expense".to_owned(),
                member_id: "synthetic-member-b".to_owned(),
                paid_minor: 0,
                share_minor: 5_000,
            },
            HouseholdObligationEntry {
                obligation_id: "refund".to_owned(),
                member_id: "synthetic-member-a".to_owned(),
                paid_minor: -2_000,
                share_minor: -1_000,
            },
            HouseholdObligationEntry {
                obligation_id: "refund".to_owned(),
                member_id: "synthetic-member-b".to_owned(),
                paid_minor: 0,
                share_minor: -1_000,
            },
        ],
    })?;
    assert_eq!(output.positions[0].member_id, "synthetic-member-a");
    assert_eq!(output.positions[0].net_minor, 4_000);
    assert_eq!(output.positions[1].net_minor, -4_000);
    assert_eq!(output.transfers.len(), 1);
    assert_eq!(output.transfers[0].from_member_id, "synthetic-member-b");
    assert_eq!(output.transfers[0].to_member_id, "synthetic-member-a");
    assert_eq!(output.transfers[0].amount_minor, 4_000);

    let entries = (0..=MAX_HOUSEHOLD_MEMBERS)
        .map(|index| HouseholdObligationEntry {
            obligation_id: format!("obligation-{index}"),
            member_id: format!("member-{index}"),
            paid_minor: 0,
            share_minor: 0,
        })
        .collect();
    assert!(matches!(
        calculate_household_settlement(&HouseholdSettlementInput {
            currency: Currency::Cad,
            entries,
        }),
        Err(EngineError::LimitExceeded {
            resource: "household members",
            limit: MAX_HOUSEHOLD_MEMBERS,
        })
    ));
    Ok(())
}
