from metric_analysis.get_abnormal_metrics import ks_test_filter, ttest_filter, zscore_like_filter, filter_metrics, FULL_WINDOW_SIZE, TEST_WINDOW_SIZE
import numpy as np

def test_ks_test_filter():
    l1 = [1, 1, 1, 2, 1, 1, 2, 1, 2, 1, 1]
    l2 = [2, 4, 8, 16, 32]
    assert ks_test_filter(l1, l2) == (True, 0.013736263736263736)
    assert ks_test_filter(l1, l1) == (False, 1.0)


def test_ttest_filter():
    l1 = [1, 2, 3, 4, 5, 5, 4, 3, 2, 1]
    l2 = [2, 4, 6, 8, 10]
    l3 = [2, 3, 5, 6, 3]
    assert ttest_filter(l1, l2) == (True, 0.024215051722005176)
    assert ttest_filter(l1, l3) == (False, 0.3599732823338926)


def test_zscore_like_filter():
    history = [1, 2, 3, 4, 5, 5, 4, 3, 2, 1]
    test_window1 = [6, 7, 10, 8, 7]
    test_window2 = [0, 1, 2, 3, 4]
    assert zscore_like_filter(history, test_window1) == True
    assert zscore_like_filter(history, test_window2) == False


def test_filter_metrics():
    v1_head = np.random.normal(loc=10, scale=2, size=FULL_WINDOW_SIZE-TEST_WINDOW_SIZE)
    v1_tail = np.random.normal(loc=30, scale=3, size=TEST_WINDOW_SIZE)
    v1 = np.concatenate([v1_head, v1_tail])
    values1 = [[i, v] for i, v in enumerate(v1)]
    v2 = np.random.normal(loc=10, scale=2, size=FULL_WINDOW_SIZE)
    values2 = [[i, v] for i, v in enumerate(v2)]
    query_results = {
        "result": [
            {
                "metric": {"pod": "pod1"},
                "values": values1,
            },
            {
                "metric": {"pod": "pod2"},
                "values": values2,
            },
        ]
    }
    metric_name = "metric1"
    abnormal, total = filter_metrics(query_results, metric_name)
    assert len(abnormal) == 1
    assert abnormal[0][0] ==  "pod1"
    assert abnormal[0][1] == "metric1"
    assert total == [("pod1", "metric1"), ("pod2", "metric1")]