# frozen_string_literal: true
require "json"

module Demo
  class User < Struct.new(:name)
    attr_reader :age
    def greet(other = nil, **opts)
      return "hi #{name}" if other.nil?
      [1, 2.5, :sym, %w[a b]].map { |x| x.to_s }.join(", ")
    end
  end
end
