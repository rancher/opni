# Standard Library
from datetime import datetime, timedelta
from typing import List, Union

# Third Party
import numpy as np
from access_admin_api import list_all_metric, metric_queryrange
from model.utils import plt_plot
from scipy.stats import ks_2samp, ttest_ind

PVALUE_THRESHOLD = 0.05
STD_MULTIPLIER = 3
FULL_WINDOW_SIZE = 360  # unit : minute
HISTORY_WINDOW_SIZE = 300
PATTERN_WINDOW_SIZE = 60  # window size for pattern match
TEST_WINDOW_SIZE = 10  # test anomaly in this window


def ks_test_filter(l1: Union[List[float], np.array], l2: Union[List[float], np.array]):
    """
    2 sample ks-test used in paper
    return: True if the 2 samples look significantly different. else False
    """
    _, p_value = ks_2samp(l1, l2)
    if p_value < PVALUE_THRESHOLD:
        return True, p_value
    else:
        return False, p_value


def ttest_filter(l1: Union[List[float], np.array], l2: Union[List[float], np.array]):
    """
    2 sample T-test to compare if l2 has a significant shift of mean from l1
    return: True if the 2 samples look significantly different in terms of mean. else False
    """
    _, p_value = ttest_ind(l1, l2)
    if p_value < PVALUE_THRESHOLD:
        return True, p_value
    else:
        return False, p_value


def zscore_like_filter(history: Union[List[float], np.array], test_window: Union[List[float], np.array]) -> bool:
    """
    Use the idea of z-score to examine samples in `test_window` except using the mean and std from `history` as reference.
    return: True if test_window seems to contain outliers. else False
    """
    mean = np.mean(history)
    std_dev = np.std(history)
    std_multiplier = STD_MULTIPLIER
    rule1 = (
        max(test_window) > mean + std_multiplier * std_dev
        or min(test_window) < mean - std_multiplier * std_dev
    )
    rule2 = (
        np.mean(test_window) > mean + std_multiplier * std_dev
        or np.mean(test_window) < mean - std_multiplier * std_dev
    ) 
    if rule1 and rule2:
        return True
    else: 
        return False


def filter_metrics(query_results, metric_name, is_debug=False):
    """
    filter out anomalous metrics using an ensemble of different rules
    """
    total = []
    abnormal = []
    for r in query_results["result"]:
        # for the same metric m, each pod will have it's own time-series for m.
        # check pod-level metrics in the first iteration. 
        if "pod" not in r["metric"]:
            continue
        pod = r["metric"]["pod"]
        values0 = r["values"]
        values0 = [float(l[1]) for l in values0]
        history, evaluate_window, test_window = (
            values0[:HISTORY_WINDOW_SIZE],
            values0[-PATTERN_WINDOW_SIZE:-TEST_WINDOW_SIZE],
            values0[-TEST_WINDOW_SIZE:],
        )
        data_window = values0[-PATTERN_WINDOW_SIZE:]
        # TODO: check values , length of vectors before moving forward

        try:
            is_anomaly, _ = ttest_filter(evaluate_window, test_window)
            total.append((pod, metric_name))
            if is_anomaly:
                if zscore_like_filter(history, test_window):
                    abnormal.append((pod, metric_name, data_window))

        except Exception as e:
            # TODO: better handle the exceptions here
            pass
    return abnormal, total


async def pull_metrics(end_time, service, user_id: str, metrics, ns):
    """
    pull time-series of all the metrics in a namespace
    """
    res = []
    for m in metrics: # for each metric m, query metrics data
        r = await metric_queryrange(
            service,
            user_id,
            m,
            end_time=end_time,
            time_delta=timedelta(minutes=FULL_WINDOW_SIZE),
            step_minute=1,
            namespace=ns,
        )
        res.append(r)
    return res


async def get_abnormal_metrics(
    service, cluster_id, requested_ts: datetime = None, ns: str = "default"
):
    """
    main function of this script. Pull all metrics in namespace `ns` and filter out anomalous ones and return
    """
    if requested_ts is None:
        requested_ts = datetime.now()
    metrics = await list_all_metric(service, cluster_id)
    query_res = await pull_metrics(requested_ts, service, cluster_id, metrics, ns)

    abnormals = []
    totals = []
    for i, q in enumerate(query_res):
        a, t = filter_metrics(q, metrics[i])
        abnormals.extend(a)
        totals.extend(t)
    return abnormals, totals
