# Standard Library
from datetime import datetime, timedelta
from typing import List

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


def moving_average(data, window_size=2):
    data = np.array(data)

    weights = np.repeat(1.0, window_size) / window_size
    moving_avg = np.convolve(data, weights, "valid")
    return moving_avg


def ks_anomaly_detection(l1: List[float], l2: List[float]):
    # metric_values = moving_average(metric_values)
    _, p_value = ks_2samp(l1, l2)
    if p_value < PVALUE_THRESHOLD:
        return True, p_value
    else:
        return False, p_value


def ttest_anomaly_detection(l1: List[float], l2: List[float]):
    # metric_values = moving_average(metric_values)
    _, p_value = ttest_ind(l1, l2)
    if p_value < PVALUE_THRESHOLD:
        return True, p_value
    else:
        return False, p_value


def zscore_anomaly_detection(metric_values: List[float]):
    mean = np.mean(metric_values)
    std_dev = np.std(metric_values)

    # set a threshold value
    threshold = STD_MULTIPLIER

    # identify anomalies using z-score
    anomalies = []
    for x in metric_values:
        z_score = abs((x - mean) / std_dev)
        if z_score > threshold:
            anomalies.append(x)
    return anomalies


# Standard Library


async def pull_metrics_data(end_time, service, user_id: str, metrics, ns):
    """
    pull time-series of all the metrics in a namespace
    """
    res = []
    for m in metrics:
        q1 = await metric_queryrange(
            service,
            user_id,
            m,
            end_time=end_time,
            time_delta=timedelta(minutes=FULL_WINDOW_SIZE),
            step_minute=1,
            namespace=ns,
        )
        res.append(q1)
    return res


def filter_metrics(q1, m_name, is_debug=False):
    """
    filter out anomalous metrics using an ensemble of different rules
    """
    total = []
    abnormal = []
    for r in q1["result"]:
        # check pod-level metrics in the first iteration
        if "pod" not in r["metric"]:
            continue
        pod = r["metric"]["pod"]
        list0 = r["values"]
        values0 = [float(l[1]) for l in list0]
        history, evaluate_window, test_window = (
            values0[:HISTORY_WINDOW_SIZE],
            values0[-PATTERN_WINDOW_SIZE:-TEST_WINDOW_SIZE],
            values0[-TEST_WINDOW_SIZE:],
        )
        data_window = values0[-PATTERN_WINDOW_SIZE:]
        try:
            is_anomaly, _ = ttest_anomaly_detection(evaluate_window, test_window)
            total.append((pod, m_name))
            if is_anomaly:
                mean = np.mean(history)
                std_dev = np.std(history)
                std_multiplier = STD_MULTIPLIER
                rule1 = (
                    max(test_window) > mean + std_multiplier * std_dev
                    or min(test_window) < mean - std_multiplier * std_dev
                )
                rule2 = max(test_window) > max(history) or min(test_window) < min(
                    history
                )
                rule3 = (
                    np.mean(test_window) > mean + std_multiplier * std_dev
                    or np.mean(test_window) < mean - std_multiplier * std_dev
                )
                if rule1 and rule3:
                    abnormal.append((pod, m_name, data_window))
                    if is_debug:
                        print(m_name)
                        print(pod)
                        plt_plot(np.array(values0))
                        print("=====================")
        except Exception as e:
            # TODO: better handle the exceptions here
            pass
    return abnormal, total


async def get_abnormal_metrics(
    service, cluster_id, requested_ts: datetime = None, ns: str = "default"
):
    """
    main function of this script. Pull all metrics in namespace `ns` and filter out anomalous ones and return
    """
    if requested_ts is None:
        requested_ts = datetime.now()
    metrics = await list_all_metric(service, cluster_id)
    qs = await pull_metrics_data(requested_ts, service, cluster_id, metrics, ns)

    abnormals = []
    totals = []
    for i, q in enumerate(qs):
        a, t = filter_metrics(q, metrics[i])
        abnormals.extend(a)
        totals.extend(t)
    return abnormals, totals
