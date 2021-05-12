"""
Description : This file implements the Drain algorithm for log parsing
Author      : LogPAI team
Modified by : david.ohana@ibm.com, moshikh@il.ibm.com
License     : MIT
"""
from typing import List, Dict

from cachetools import LRUCache, Cache

from drain3.simple_profiler import Profiler, NullProfiler


class LogCluster:
    __slots__ = ["log_template_tokens", "cluster_id", "size"]

    def __init__(self, log_template_tokens: list, cluster_id: int):
        self.log_template_tokens = tuple(log_template_tokens)
        self.cluster_id = cluster_id
        self.size = 1

    def get_template(self):
        return ' '.join(self.log_template_tokens)

    def __str__(self):
        return f"ID={str(self.cluster_id).ljust(5)} : size={str(self.size).ljust(10)}: {self.get_template()}"


class Node:
    __slots__ = ["key_to_child_node", "cluster_ids"]

    def __init__(self):
        self.key_to_child_node: Dict[str, Node] = {}
        self.cluster_ids: List[int] = []


class Drain:
    def __init__(self,
                 depth=4,
                 sim_th=0.4,
                 max_children=100,
                 max_clusters=None,
                 extra_delimiters=(),
                 profiler: Profiler = NullProfiler(),
                 param_str="<*>"):
        """
        Attributes
        ----------
            depth : depth of all leaf nodes (nodes that contain log clusters)
            sim_th : similarity threshold - if percentage of similar tokens for a log message is below this
                number, a new log cluster will be created.
            max_children : max number of children of an internal node
            max_clusters : max number of tracked clusters (unlimited by default).
                When this number is reached, model starts replacing old clusters
                with a new ones according to the LRU policy.
            extra_delimiters: delimiters to apply when splitting log message into words (in addition to whitespace).
        """
        self.depth = depth - 2  # number of prefix tokens in each tree path (exclude root and leaf node)
        self.sim_th = sim_th
        self.max_children = max_children
        self.root_node = Node()
        self.profiler = profiler
        self.extra_delimiters = extra_delimiters
        self.max_clusters = max_clusters
        self.param_str = param_str

        # key: int, value: LogCluster
        self.id_to_cluster = {} if max_clusters is None else LRUCache(maxsize=max_clusters)
        self.clusters_counter = 0

    @property
    def clusters(self):
        return self.id_to_cluster.values()

    @staticmethod
    def has_numbers(s):
        return any(char.isdigit() for char in s)

    def tree_search(self, root_node: Node, tokens: list, sim_th: float, include_params: bool):

        # at first level, children are grouped by token (word) count
        token_count = len(tokens)
        parent_node = root_node.key_to_child_node.get(str(token_count))

        # no template with same token count yet
        if parent_node is None:
            return None

        # handle case of empty log string - return the single cluster in that group
        if token_count == 0:
            return self.id_to_cluster.get(parent_node.cluster_ids[0])

        # find the leaf node for this log - a path of nodes matching the first N tokens (N=tree depth)
        current_depth = 1
        for token in tokens:
            # at_max_depth
            if current_depth == self.depth:
                break

            # is_last_token
            if current_depth == token_count:
                break

            key_to_child_node = parent_node.key_to_child_node
            parent_node = key_to_child_node.get(token)
            if parent_node is None:
                parent_node = key_to_child_node.get(self.param_str)
            if parent_node is None:
                return None

            current_depth += 1

        # get best match among all clusters with same prefix, or None if no match is above sim_th
        cluster = self.fast_match(parent_node.cluster_ids, tokens, sim_th, include_params)
        return cluster

    def add_seq_to_prefix_tree(self, root_node, cluster: LogCluster):
        token_count_str = str(len(cluster.log_template_tokens))
        if token_count_str not in root_node.key_to_child_node:
            first_layer_node = Node()
            root_node.key_to_child_node[token_count_str] = first_layer_node
        else:
            first_layer_node = root_node.key_to_child_node[token_count_str]

        parent_node = first_layer_node

        # handle case of empty log string
        if len(cluster.log_template_tokens) == 0:
            parent_node.cluster_ids = [cluster.cluster_id]
            return

        current_depth = 1
        for token in cluster.log_template_tokens:

            # if at max depth or this is last token in template - add current log cluster to the leaf node
            if current_depth == self.depth or str(current_depth) == token_count_str:
                # clean up stale clusters before adding a new one.
                new_cluster_ids = [cluster.cluster_id]
                for cluster_id in parent_node.cluster_ids:
                    if cluster_id in self.id_to_cluster:
                        new_cluster_ids.append(cluster_id)
                parent_node.cluster_ids = new_cluster_ids
                break

            # if token not matched in this layer of existing tree.
            if token not in parent_node.key_to_child_node:
                if not self.has_numbers(token):
                    if self.param_str in parent_node.key_to_child_node:
                        if len(parent_node.key_to_child_node) < self.max_children:
                            new_node = Node()
                            parent_node.key_to_child_node[token] = new_node
                            parent_node = new_node
                        else:
                            parent_node = parent_node.key_to_child_node[self.param_str]
                    else:
                        if len(parent_node.key_to_child_node) + 1 < self.max_children:
                            new_node = Node()
                            parent_node.key_to_child_node[token] = new_node
                            parent_node = new_node
                        elif len(parent_node.key_to_child_node) + 1 == self.max_children:
                            new_node = Node()
                            parent_node.key_to_child_node[self.param_str] = new_node
                            parent_node = new_node
                        else:
                            parent_node = parent_node.key_to_child_node[self.param_str]

                else:
                    if self.param_str not in parent_node.key_to_child_node:
                        new_node = Node()
                        parent_node.key_to_child_node[self.param_str] = new_node
                        parent_node = new_node
                    else:
                        parent_node = parent_node.key_to_child_node[self.param_str]

            # if the token is matched
            else:
                parent_node = parent_node.key_to_child_node[token]

            current_depth += 1

    # seq1 is template
    def get_seq_distance(self, seq1, seq2, include_params: bool):
        assert len(seq1) == len(seq2)
        sim_tokens = 0
        param_count = 0

        for token1, token2 in zip(seq1, seq2):
            if token1 == self.param_str:
                param_count += 1
                continue
            if token1 == token2:
                sim_tokens += 1

        if include_params:
            sim_tokens += param_count

        ret_val = float(sim_tokens) / len(seq1)

        return ret_val, param_count

    def fast_match(self, cluster_ids: list, tokens: list, sim_th: float, include_params: bool):
        """
        Find the best match for a log message (represented as tokens) versus a list of clusters
        :param cluster_ids: List of clusters to match against (represented by their IDs)
        :param tokens: the log message, separated to tokens.
        :param sim_th: minimum required similarity threshold (None will be returned in no clusters reached it)
        :param include_params: consider tokens matched to wildcard parameters in similarity treshold.
        :return: Best match cluster or None
        """
        match_cluster = None

        max_sim = -1
        max_param_count = -1
        max_cluster = None

        for cluster_id in cluster_ids:
            # Try to retrieve cluster from cache with bypassing eviction
            # algorithm as we are only testing candidates for a match.
            cluster = Cache.get(self.id_to_cluster, cluster_id)
            if cluster is None:
                continue
            cur_sim, param_count = self.get_seq_distance(cluster.log_template_tokens, tokens, include_params)
            if cur_sim > max_sim or (cur_sim == max_sim and param_count > max_param_count):
                max_sim = cur_sim
                max_param_count = param_count
                max_cluster = cluster

        if max_sim >= sim_th:
            match_cluster = max_cluster

        return match_cluster

    def create_template(self, seq1, seq2):
        assert len(seq1) == len(seq2)
        ret_val = list(seq2)

        for i, (token1, token2) in enumerate(zip(seq1, seq2)):
            if token1 != token2:
                ret_val[i] = self.param_str

        return ret_val

    def print_tree(self, file=None):
        self.print_node("root", self.root_node, 0, file)

    def print_node(self, token, node, depth, file):
        out_str = '\t' * depth

        if depth < 2:
            out_str += '<' + str(token) + '>'
        else:
            out_str += token

        print(out_str, file=file)

        for token, child in node.key_to_child_node.items():
            self.print_node(token, child, depth + 1, file)

    def get_content_as_tokens(self, content):
        content = content.strip()
        for delimiter in self.extra_delimiters:
            content = content.replace(delimiter, " ")
        content_tokens = content.split()
        return content_tokens

    def add_log_message(self, content: str):
        content_tokens = self.get_content_as_tokens(content)

        if self.profiler:
            self.profiler.start_section("tree_search")
        match_cluster = self.tree_search(self.root_node, content_tokens, self.sim_th, False)
        if self.profiler:
            self.profiler.end_section()

        # Match no existing log cluster
        if match_cluster is None:
            if self.profiler:
                self.profiler.start_section("create_cluster")
            self.clusters_counter += 1
            cluster_id = self.clusters_counter
            match_cluster = LogCluster(content_tokens, cluster_id)
            self.id_to_cluster[cluster_id] = match_cluster
            self.add_seq_to_prefix_tree(self.root_node, match_cluster)
            update_type = "cluster_created"

        # Add the new log message to the existing cluster
        else:
            if self.profiler:
                self.profiler.start_section("cluster_exist")
            new_template_tokens = self.create_template(content_tokens, match_cluster.log_template_tokens)
            if tuple(new_template_tokens) == match_cluster.log_template_tokens:
                update_type = "none"
            else:
                match_cluster.log_template_tokens = tuple(new_template_tokens)
                update_type = "cluster_template_changed"
            match_cluster.size += 1
            # Touch cluster to update its state in the cache.
            self.id_to_cluster.get(match_cluster.cluster_id)

        if self.profiler:
            self.profiler.end_section()

        return match_cluster, update_type

    def match(self, content: str):
        """
        Match against an already existing cluster. Match shall be perfect (sim_th=1.0).
        New cluster will not be created as a result of this call, nor any cluster modifications.
        :param content: log message to match
        :return: Matched cluster or None of no match found.
        """
        content_tokens = self.get_content_as_tokens(content)
        match_cluster = self.tree_search(self.root_node, content_tokens, 1.0, True)
        return match_cluster

    def get_total_cluster_size(self):
        size = 0
        for c in self.id_to_cluster.values():
            size += c.size
        return size
