from metric_analysis.model.metric_pattern_classification import eval_model, predict, class_map
import numpy as np

test_data = np.loadtxt("tests/test-data/test_array.txt")
test_label = np.loadtxt("tests/test-data/test_label.txt")


def test_predict():
    preds = predict(test_data)
    str_test_label = [class_map[l] for l in test_label]
    for (p, l) in zip(preds, str_test_label):
        assert p == l


def test_eval_model():
    accuracy = eval_model(test_x=test_data, test_y=test_label)
    assert accuracy == 1.0
