from scipy.stats import ks_2samp, ttest_ind
from collections import defaultdict
from model.data_simulator import plt_plot
from access_admin_api import metric_queryrange, list_all_metric
import numpy as np
from datetime import datetime, timedelta
from typing import List


threshold = 0.05

def moving_average(data, window_size=2):
    data = np.array(data)

    weights = np.repeat(1.0, window_size) / window_size
    moving_avg = np.convolve(data, weights, 'valid')
    return moving_avg

def ks_anomaly_detection(l1: List[float], l2: List[float]):
    # metric_values = moving_average(metric_values)
    ks_stat, p_value = ks_2samp(l1, l2)
    if p_value < threshold:
        return True, p_value
    else:
        return False, p_value
    

def ttest_anomaly_detection(l1:List[float], l2: List[float]):
    # metric_values = moving_average(metric_values)
    stat, p_value = ttest_ind(l1, l2)
    if p_value < threshold:
        return True, p_value
    else:
        return False, p_value

def zscore_anomaly_detection(metric_values: List[float]):
    mean = np.mean(metric_values)
    std_dev = np.std(metric_values)

    # set a threshold value
    threshold = 5

    # identify anomalies using z-score
    anomalies = []
    for x in metric_values:
        z_score = abs((x - mean) / std_dev)
        if z_score > threshold:
            anomalies.append(x)
    return anomalies

import time 


async def pull_metrics_data(end_time, service, user_id:str, metrics, ns="opni"):
    res = []
    for m in metrics:
        q1 = await metric_queryrange(service, user_id, m,end_time=end_time, time_delta=timedelta(minutes=300),step_minute=1, namespace=ns)
        res.append(q1)
    return res


def filter_metrics(d, q1 ,m_name, is_debug = False):
    count = 0
    total = 0
    res = []
    for r in q1["result"]:
        if "pod" not in r["metric"]:
            # print(r)
            continue
        pod = r["metric"]["pod"]
        list0 = r["values"]
        values0 = [float(l[1]) for l in list0]
        history, evaluate_window, test_window = values0[:240], values0[-60:-10], values0[-10:]
        data_window = values0[-60:]
        try:
            is_anomaly, p_value = ttest_anomaly_detection(evaluate_window, test_window)
            total += 1
            if is_anomaly:
                mean = np.mean(history)
                std_dev = np.std(history)
                std_multiplier = 3
                rule1 = max(test_window) > mean + std_multiplier * std_dev or min(test_window) < mean - std_multiplier * std_dev
                rule2 = max(test_window) > max(history) or min(test_window) < min(history)
                rule3 = np.mean(test_window) > mean + std_multiplier * std_dev or np.mean(test_window) < mean - std_multiplier * std_dev
                # _, rule4 = ks_anomaly_detection(values0, cut=150)
                if rule1 and rule3:
                    count += 1
                    d[pod  + "-" + m_name] = history
                    res.append((pod, m_name, data_window))
                    if is_debug:
                    # if True:
                        print(m_name)
                        print(pod)
                        plt_plot(np.array(values0))
                        print("=====================")
        except Exception as e:
            pass
    s2 = time.time()
    return res


async def get_abnormal_metrics(service, user_id, requested_ts: datetime= None, ns:str="default"):
    if requested_ts is None:
        requested_ts = datetime.now()
    metrics = await list_all_metric(service, user_id)
    qs = await pull_metrics_data(requested_ts, service, user_id, metrics, ns=ns)
    d = defaultdict(list)

    res = []
    for i,q in enumerate(qs):
        r = filter_metrics(d,q, metrics[i])
        res.extend(r)
    return res